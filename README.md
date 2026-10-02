# hcx — howls-codex, Codex CLI account switcher

Chạy nhiều account ChatGPT cho Codex CLI song song. Mỗi account = 1 thư mục (`CODEX_HOME`),
credential nằm trong `<dir>/auth.json` nên tự tách theo thư mục. Clone của `hcc` (howls-claude).

- `main` = `~/.codex`, `<name>` = `~/.codex-<name>`
- Account phụ symlink từ `~/.codex`: `AGENTS.md config.toml hooks.json skills rules plugins prompts history.jsonl sessions`
  (dùng chung config/skill/plugin/lịch sử, `codex resume` được session của account khác)
- Riêng từng account: `auth.json` (login), `*.sqlite`, `cache`, `memories`, ...

## Cài

```sh
go build -o ~/.local/bin/hcx .
```

## Dùng

```sh
hcx add work              # tạo/đồng bộ ~/.codex-work; item thật cũ -> <item>.hcx-bak-<unix>
hcx work                  # chạy codex bằng account work (lần đầu: hcx work login)
hcx main resume --last    # args sau tên account chuyển thẳng cho codex
hcx default work          # set account mặc định (lưu file .hcx-default); không tên = in default
hcx -c                    # không tên / args bắt đầu bằng - => chạy account mặc định
hcx list                  # * = account theo $CODEX_HOME hiện tại
hcx quota [name]          # biểu đồ quota 5h/7 ngày + giờ reset (không name = mọi account)
eval "$(hcx env work)"    # set env cho shell hiện tại
eval "$(hcx alias)"       # nạp alias cho shell hiện tại (hoặc: hcx alias >> ~/.zshrc)
```

`hcx quota` gọi `https://chatgpt.com/backend-api/wham/usage` bằng token trong `auth.json` (không tự refresh;
token hết hạn thì chạy `hcx <name>` một lần để codex refresh). Thanh 20 ô mỗi cửa sổ, màu ANSI chuẩn
(xanh < 50%, vàng < 80%, đỏ ≥ 80%), tự tắt khi pipe hoặc `NO_COLOR` được set:

```
main       a@x.com [plus]
  5h         ███░░░░░░░░░░░░░░░░░  13%  reset Thu 09:59
  7d         █████████████░░░░░░░  66%  reset Mon 17:00
```

Alias gợi ý trong `~/.zshrc`: thêm bằng `hcx alias >> ~/.zshrc` hoặc nạp bằng `eval "$(hcx alias)"`.

```sh
alias codex='hcx --sandbox workspace-write --ask-for-approval on-request'
alias codex-work='hcx work --sandbox workspace-write --ask-for-approval on-request'
```

`$CODEX_ACCOUNT` được set khi chạy, statusline/hook có thể đọc để hiện account.

Test: `go test ./...`
