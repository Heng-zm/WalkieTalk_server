# Validation notes

Performed in this ChatGPT container after the frontend brand/channel-type update:

- `gofmt -w $(find backend -name '*.go')` completed successfully.
- Frontend inline JavaScript was extracted from `frontend/index.html` and checked with:

```bash
node --check /mnt/data/frontend-inline-brand-channel.js
```

Result: JavaScript syntax check passed.

- Go package compile/test was checked with local stubs for external modules because this container cannot reach `proxy.golang.org`:

```bash
go mod edit -replace github.com/gorilla/websocket=/mnt/data/go_stubs/websocket
go mod edit -replace github.com/redis/go-redis/v9=/mnt/data/go_stubs/redis
go test ./...
```

Result: all packages compiled and `go test ./...` passed with no test files.

Not completed in this container:

- A real internet-backed `go mod download`, `go test ./...`, and `go build ./cmd/server` could not be performed because DNS access to `proxy.golang.org` is blocked in this container.

Run this on Render or any machine with internet access:

```bash
cd backend
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Frontend update validated:

- Website/PWA title and manifest name changed to `អាយកូម`.
- Header now shows the `អាយកូម` brand name next to the app mark.
- Khmer-friendly Google font stack added with `Noto Sans Khmer`.
- Channel sheet now lets users select `សាធារណៈ / Public` or `ឯកជន / Private`.
- Channel list displays public/private labels and `ចំនួនមនុស្ស` count.
- Join/create sends `visibility` and `channel_type` to the backend.

Backend channel metadata update validated:

- `ChannelState` now includes `visibility`.
- `/channels` includes each channel visibility.
- `join_room` preserves channel visibility metadata for the channel list.

Previous fixes retained:

- Message acknowledgement events are supported and no longer reported as `UNKNOWN_EVENT`.
- `/webhook/keepalive` and `/hooks/keepalive` endpoints remain available.
- `/health` remains lightweight and `/ready` remains cached by `READINESS_CACHE_SECS`.
