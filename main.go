// hcx: chạy nhiều account Codex CLI song song bằng CODEX_HOME.
// main = ~/.codex, <name> = ~/.codex-<name>. Credential nằm trong <dir>/auth.json nên tự tách theo dir.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Item symlink từ ~/.codex sang account phụ.
var shared = []string{"AGENTS.md", "config.toml", "hooks.json", "skills", "rules", "plugins", "prompts", "history.jsonl", "sessions"}

// Item nguồn chưa có thì tạo dạng thư mục (còn lại tạo file rỗng).
var sharedDirs = map[string]bool{"skills": true, "rules": true, "plugins": true, "prompts": true, "sessions": true}

const aliasFlags = "--sandbox workspace-write --ask-for-approval on-request"

const usage = `hcx — Codex account switcher

  hcx add <name>            tạo/đồng bộ ~/.codex-<name>, symlink shared từ ~/.codex
  hcx list                  liệt kê account (* = đang active theo $CODEX_HOME)
  hcx env <name>            in lệnh export, dùng: eval "$(hcx env <name>)"
  hcx alias [name]          in alias zsh gợi ý, dùng: eval "$(hcx alias)"
  hcx quota [name]          xem quota 5h/7 ngày (không name = mọi account)
  hcx <name> [args...]      chạy codex với account <name>`

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		die(err)
	}
	return h
}

func mainDir() string { return filepath.Join(home(), ".codex") }

func accountDir(name string) string {
	if name == "main" {
		return mainDir()
	}
	return filepath.Join(home(), ".codex-"+name)
}

func die(v any) {
	fmt.Fprintln(os.Stderr, "hcx:", v)
	os.Exit(1)
}

func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\. `) && !strings.HasPrefix(name, "-")
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(usage)
		return
	}
	var err error
	switch args[0] {
	case "add":
		if len(args) != 2 {
			die("usage: hcx add <name>")
		}
		err = cmdAdd(args[1])
	case "list", "ls":
		err = cmdList()
	case "env":
		if len(args) != 2 {
			die("usage: hcx env <name>")
		}
		err = cmdEnv(args[1])
	case "alias":
		if len(args) > 2 {
			die("usage: hcx alias [name]")
		}
		err = cmdAlias(args[1:])
	case "quota":
		err = cmdQuota(args[1:])
	default:
		err = run(args[0], args[1:])
	}
	if err != nil {
		die(err)
	}
}

func cmdAdd(name string) error {
	if !validName(name) || name == "main" {
		return fmt.Errorf("tên không hợp lệ: %q", name)
	}
	dir := accountDir(name)
	if err := os.MkdirAll(mainDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, item := range shared {
		if err := linkShared(item, dir); err != nil {
			return fmt.Errorf("%s: %w", item, err)
		}
	}
	fmt.Printf("OK: %s. Login: hcx %s\n", dir, name)
	return nil
}

// linkShared: dir/item -> ~/.codex/item. Idempotent; item thật bị rename sang .hcx-bak-<unix>, không xoá.
func linkShared(item, dir string) error {
	src := filepath.Join(mainDir(), item)
	dst := filepath.Join(dir, item)
	if _, err := os.Lstat(src); os.IsNotExist(err) {
		if sharedDirs[item] {
			err = os.MkdirAll(src, 0o755)
		} else {
			err = os.WriteFile(src, nil, 0o644)
		}
		if err != nil {
			return err
		}
	}
	if cur, err := os.Readlink(dst); err == nil && cur == src {
		return nil
	}
	if _, err := os.Lstat(dst); err == nil {
		bak := fmt.Sprintf("%s.hcx-bak-%d", dst, time.Now().Unix())
		if err := os.Rename(dst, bak); err != nil {
			return err
		}
		fmt.Printf("backup: %s -> %s\n", dst, bak)
	}
	return os.Symlink(src, dst)
}

func accounts() []string {
	names := []string{"main"}
	matches, _ := filepath.Glob(filepath.Join(home(), ".codex-*"))
	for _, m := range matches {
		// Account = đã login (auth.json) hoặc tạo bởi hcx (config.toml là symlink).
		_, login := os.Stat(filepath.Join(m, "auth.json"))
		fi, link := os.Lstat(filepath.Join(m, "config.toml"))
		if login == nil || (link == nil && fi.Mode()&os.ModeSymlink != 0) {
			names = append(names, strings.TrimPrefix(filepath.Base(m), ".codex-"))
		}
	}
	return names
}

type tokens struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id"`
}

// readAuth: tokens ChatGPT login trong <dir>/auth.json.
func readAuth(dir string) (tokens, error) {
	var a struct {
		Tokens tokens `json:"tokens"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err == nil {
		err = json.Unmarshal(b, &a)
	}
	return a.Tokens, err
}

// jwtClaims: decode payload JWT (không verify chữ ký, chỉ để đọc).
func jwtClaims(tok string, v any) error {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return fmt.Errorf("JWT không hợp lệ")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func email(dir string) string {
	t, err := readAuth(dir)
	var c struct {
		Email string `json:"email"`
	}
	if err != nil || jwtClaims(t.IDToken, &c) != nil || c.Email == "" {
		return "(chưa login)"
	}
	return c.Email
}

func cmdList() error {
	active := filepath.Clean(os.Getenv("CODEX_HOME"))
	for _, name := range accounts() {
		dir := accountDir(name)
		mark := " "
		if dir == active {
			mark = "*"
		}
		fmt.Printf("%s %-10s %-30s %s\n", mark, name, email(dir), dir)
	}
	return nil
}

func resolve(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("tên không hợp lệ: %q\n%s", name, usage)
	}
	dir := accountDir(name)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("account %q chưa có, chạy: hcx add %s", name, name)
	}
	return dir, nil
}

func cmdEnv(name string) error {
	dir, err := resolve(name)
	if err != nil {
		return err
	}
	fmt.Printf("export CODEX_HOME=%q CODEX_ACCOUNT=%q\n", dir, name)
	return nil
}

func aliasLine(name string) string {
	aliasName := "codex"
	if name != "main" {
		aliasName = "codex-" + name
	}
	return fmt.Sprintf("alias %s='hcx %s %s'", aliasName, name, aliasFlags)
}

func cmdAlias(names []string) error {
	if len(names) == 0 {
		for _, name := range accounts() {
			fmt.Println(aliasLine(name))
		}
		return nil
	}
	if _, err := resolve(names[0]); err != nil {
		return err
	}
	fmt.Println(aliasLine(names[0]))
	return nil
}

func run(name string, args []string) error {
	dir, err := resolve(name)
	if err != nil {
		return err
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		return err
	}
	env := []string{"CODEX_HOME=" + dir, "CODEX_ACCOUNT=" + name}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CODEX_HOME=") && !strings.HasPrefix(kv, "CODEX_ACCOUNT=") {
			env = append(env, kv)
		}
	}
	return syscall.Exec(bin, append([]string{"codex"}, args...), env)
}

// token: access token + account id từ auth.json. Không tự refresh (codex sẽ refresh khi chạy).
func token(name, dir string) (tokens, error) {
	t, err := readAuth(dir)
	if err != nil || t.AccessToken == "" {
		return t, fmt.Errorf("chưa login, chạy: hcx %s", name)
	}
	var c struct {
		Exp int64 `json:"exp"`
	}
	if jwtClaims(t.AccessToken, &c) == nil && c.Exp < time.Now().Unix() {
		return t, fmt.Errorf("token hết hạn, chạy: hcx %s", name)
	}
	return t, nil
}

func fetchUsage(t tokens) ([]byte, error) {
	req, err := http.NewRequest("GET", "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("ChatGPT-Account-Id", t.AccountID)
	req.Header.Set("User-Agent", "codex")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API: %s", resp.Status)
	}
	return body, nil
}

// bar: thanh 20 ô cho pct%. color = 16 màu ANSI chuẩn nên tự khớp theme terminal.
func bar(pct float64, color bool) string {
	n := min(max(int(math.Round(pct/5)), 0), 20)
	full, empty := strings.Repeat("█", n), strings.Repeat("░", 20-n)
	if !color {
		return full + empty
	}
	c := "32" // green
	if pct >= 80 {
		c = "31" // red
	} else if pct >= 50 {
		c = "33" // yellow
	}
	return "\x1b[" + c + "m" + full + "\x1b[0;2m" + empty + "\x1b[0m"
}

// windowLabel: 18000s -> 5h, 604800s -> 7d.
func windowLabel(sec int64) string {
	m := sec / 60
	switch {
	case m > 0 && m%1440 == 0:
		return fmt.Sprintf("%dd", m/1440)
	case m > 0 && m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dm", m)
}

// formatUsage: trả về plan_type + các dòng thanh quota (schema RateLimitStatusPayload của codex).
func formatUsage(body []byte, color bool) (string, string, error) {
	type window struct {
		UsedPercent        float64 `json:"used_percent"`
		LimitWindowSeconds int64   `json:"limit_window_seconds"`
		ResetAt            int64   `json:"reset_at"`
	}
	var u struct {
		PlanType  string `json:"plan_type"`
		RateLimit *struct {
			Primary   *window `json:"primary_window"`
			Secondary *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return "", "", err
	}
	if u.RateLimit == nil {
		return u.PlanType, "  (không có rate limit)", nil
	}
	var lines []string
	for _, w := range []*window{u.RateLimit.Primary, u.RateLimit.Secondary} {
		if w == nil {
			continue
		}
		l := fmt.Sprintf("  %-10s %s %3.0f%%", windowLabel(w.LimitWindowSeconds), bar(w.UsedPercent, color), w.UsedPercent)
		if w.ResetAt > 0 {
			l += "  reset " + time.Unix(w.ResetAt, 0).Local().Format("Mon 15:04")
		}
		lines = append(lines, l)
	}
	return u.PlanType, strings.Join(lines, "\n"), nil
}

func quota(name, dir string, color bool) (string, string, error) {
	t, err := token(name, dir)
	if err != nil {
		return "", "", err
	}
	body, err := fetchUsage(t)
	if err != nil {
		return "", "", err
	}
	return formatUsage(body, color)
}

func cmdQuota(names []string) error {
	if len(names) == 0 {
		names = accounts()
	}
	fi, _ := os.Stdout.Stat()
	color := fi != nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
	for _, name := range names {
		dir, err := resolve(name)
		if err != nil {
			return err
		}
		plan, out, err := quota(name, dir, color)
		if err != nil {
			out = "  lỗi: " + err.Error()
		}
		if plan != "" {
			plan = " [" + plan + "]"
		}
		fmt.Printf("%-10s %s%s\n%s\n", name, email(dir), plan, out)
	}
	return nil
}
