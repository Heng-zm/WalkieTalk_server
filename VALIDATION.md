# Validation

This package was patched after the Render error:

```text
missing go.sum entry for module providing package github.com/gorilla/websocket
missing go.sum entry for module providing package github.com/redis/go-redis/v9
```

## What was changed

1. `Dockerfile` now runs `go mod tidy` after copying the full source tree and before `go build`.
2. Empty `go.sum` is included so local tooling has the expected file.
3. Fixed a Go compile issue in `internal/realtime/screen.go` where `s := ...` was missing inside `anyBool()`.
4. Fixed WebSocket lifecycle in `internal/api/server.go` by not using `r.Context()` after the WebSocket upgrade.

## Validate locally

```bash
go mod tidy
gofmt -w cmd/server/main.go internal/**/*.go
go test ./...
go build -trimpath -ldflags="-s -w" -o walkietalk-go ./cmd/server
```

## Validate Docker build

```bash
docker build --no-cache -t walkietalk-go .
docker run --rm -p 3000:3000 --env-file .env walkietalk-go
```

Then open:

```text
http://localhost:3000/health
http://localhost:3000/web/index.html
```
