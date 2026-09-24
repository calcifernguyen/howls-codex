package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddIdempotentWithBackup(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	os.MkdirAll(filepath.Join(h, ".codex", "skills"), 0o755)
	os.WriteFile(filepath.Join(h, ".codex", "config.toml"), []byte("x"), 0o644)
	acc := filepath.Join(h, ".codex-work")
	os.MkdirAll(acc, 0o755)
	os.WriteFile(filepath.Join(acc, "config.toml"), []byte("old"), 0o644) // file thật phải bị backup

	for i := 0; i < 2; i++ {
		if err := cmdAdd("work"); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range shared {
		got, err := os.Readlink(filepath.Join(acc, item))
		if want := filepath.Join(h, ".codex", item); err != nil || got != want {
			t.Errorf("%s: link=%q err=%v, want %q", item, got, err, want)
		}
	}
	baks, _ := filepath.Glob(filepath.Join(acc, "config.toml.hcx-bak-*"))
	if len(baks) != 1 {
		t.Errorf("want 1 backup, got %v", baks)
	}
	os.MkdirAll(filepath.Join(h, ".codex-other"), 0o755) // không auth.json, không symlink -> bỏ qua
	if got := accounts(); len(got) != 2 || got[1] != "work" {
		t.Errorf("accounts = %v", got)
	}
	if cmdAdd("main") == nil || cmdAdd("../x") == nil {
		t.Error("tên không hợp lệ phải lỗi")
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"work": true, "w2": true, "": false, "-x": false, "a/b": false, "a.b": false, "a b": false} {
		if validName(name) != want {
			t.Errorf("validName(%q) != %v", name, want)
		}
	}
}

func fakeJWT(payload string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func TestEmailAndToken(t *testing.T) {
	dir := t.TempDir()
	if got := email(dir); got != "(chưa login)" {
		t.Errorf("no auth: %q", got)
	}
	auth := `{"tokens":{"id_token":"` + fakeJWT(`{"email":"a@x.com"}`) + `","access_token":"` + fakeJWT(`{"exp":1}`) + `","account_id":"acc"}}`
	os.WriteFile(filepath.Join(dir, "auth.json"), []byte(auth), 0o600)
	if got := email(dir); got != "a@x.com" {
		t.Errorf("email = %q", got)
	}
	if _, err := token("w", dir); err == nil || !strings.Contains(err.Error(), "hết hạn") {
		t.Errorf("token expired err = %v", err)
	}
}

func TestFormatUsage(t *testing.T) {
	body := `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,` +
		`"primary_window":{"used_percent":12,"limit_window_seconds":18000,"reset_after_seconds":100,"reset_at":0},` +
		`"secondary_window":{"used_percent":40,"limit_window_seconds":604800,"reset_after_seconds":100,"reset_at":1790586000}},` +
		`"credits":{"has_credits":false,"unlimited":false,"balance":"0"}}`
	plan, got, err := formatUsage([]byte(body), false)
	if err != nil {
		t.Fatal(err)
	}
	want := "  5h         ██░░░░░░░░░░░░░░░░░░  12%\n" +
		"  7d         ████████░░░░░░░░░░░░  40%  reset " + time.Unix(1790586000, 0).Local().Format("Mon 15:04")
	if plan != "plus" || got != want {
		t.Errorf("plan %q got %q, want %q", plan, got, want)
	}
}

func TestBar(t *testing.T) {
	for pct, n := range map[float64]int{0: 0, -5: 0, 100: 20, 150: 20, 12.4: 2} {
		if got := bar(pct, false); got != strings.Repeat("█", n)+strings.Repeat("░", 20-n) {
			t.Errorf("bar(%v) = %q", pct, got)
		}
	}
	if got := bar(90, true); !strings.HasPrefix(got, "\x1b[31m") {
		t.Errorf("bar(90) màu = %q", got)
	}
}
