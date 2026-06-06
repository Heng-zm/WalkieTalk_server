# Validation notes

Performed in this ChatGPT container:

- `gofmt -w $(find internal cmd -name '*.go')` completed successfully.
- Frontend inline JavaScript was extracted from `web/index.html` and checked with:

```bash
node --check /mnt/data/frontend-inline-updated.js
```

Result: JavaScript syntax check passed.

- `go test ./...` and `go build ./cmd/server` cannot download modules directly in this container because DNS access to `proxy.golang.org` is blocked here.
- To still catch local compile errors, the same commands were run with temporary local stubs for external packages:

```bash
GOWORK=off go test -modfile=/tmp/wt_go.mod ./...
GOWORK=off go build -modfile=/tmp/wt_go.mod ./cmd/server
```

Result: compile check passed with stubs.

Run this on your machine or Render/Railway build environment with internet access:

```bash
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Feature update completed:

- Removed AI assistant from active UI.
- `/ai/chat` now returns `410 Gone` with a clear disabled message.
- Removed screen-share entry points from active UI.
- Screen-share WebSocket events now return `FEATURE_DISABLED`.
- Added `/channels` endpoint.
- Added realtime `channels_list`, `channels_state`, and `channels_expired` events.
- Added channel member counts shown as `ចំនួនមនុស្ស` in the channel sheet.
- Added 15-minute empty channel expiry via `CHANNEL_EMPTY_TTL_SECS=900`.
- Updated README, `.env.example`, and `render.yaml`.

No secret values are included in this project. Use `.env.example` and rotate any keys/passwords that were previously pasted into chat.
