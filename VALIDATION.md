# Validation notes

Performed in this ChatGPT container:

- `gofmt -w $(find . -name '*.go')` completed successfully on the Go source.
- Frontend inline JavaScript was extracted from `web/index.html` and checked with:

```bash
node --check /mnt/data/frontend-inline.js
```

Result: JavaScript syntax check passed.

- `go.sum` now includes checksums for direct Go dependencies and the go-redis runtime transitive dependencies used by this project:
  - `github.com/gorilla/websocket v1.5.3`
  - `github.com/redis/go-redis/v9 v9.7.0`
  - `github.com/cespare/xxhash/v2 v2.2.0`
  - `github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f`

Not completed in this container:

- `go test ./...` and `go build ./cmd/server` could not download modules because this container cannot reach `proxy.golang.org`.

Run this on your machine or Render/Railway build environment with internet access:

```bash
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Frontend migration completed:

- Removed Socket.IO CDN dependency.
- Added native WebSocket wrapper for `/ws`.
- Preserved the old `socket.emit(...)` and `socket.on(...)` frontend API style.
- Updated WebRTC signaling payloads to send `target_sid` for the Go backend.
- Added optional `WT_PUBLIC_API_KEY` / `wt_public_api_key` support for protected admin endpoints.

No secret values are included in this project. Use `.env.example` and rotate any keys/passwords that were previously pasted into chat.
