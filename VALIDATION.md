# Validation notes

Performed in this ChatGPT container for this update:

- `gofmt -w $(find internal cmd -name '*.go')` completed successfully.
- Frontend inline JavaScript was extracted from `web/index.html` and checked with:

```bash
node --check frontend-inline.js
```

Result: JavaScript syntax check passed.

- A compile-only Go test was run with local stub modules for `github.com/gorilla/websocket` and `github.com/redis/go-redis/v9` because this container cannot reach `proxy.golang.org`:

```bash
GOWORK=off go test -modfile=go.offline.mod ./...
```

Result: all packages compiled with no test files.

Not completed in this container:

- A real `go test ./...` / `go build ./cmd/server` using the actual upstream modules could not run here because DNS/network access to `proxy.golang.org` is blocked in this container.

Run this on your machine or Render/Railway build environment with internet access:

```bash
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Main fixes included in this version:

- Fixed `/zones` frontend calls to always send `device_id` by query string and `X-Device-Id` header.
- Changed same-origin frontend backend detection so `/web/index.html` works correctly when served by the Go backend.
- Prevented `GET /zones` without `device_id` from hard-failing with `400`; it now returns an empty device-scoped list and warning.
- Fixed zone `401 Unauthorized` for normal browser use by adding `ZONE_WRITE_REQUIRES_API_KEY=false` default. Set it to `true` only for private/admin deployments.
- Added zone read/write rate limits and zone input validation for latitude, longitude, and radius.
- Added Supabase schema compatibility for modern `radius_m`, older `radius`, and legacy table variants.
- Improved WebSocket disconnect/shutdown safety to avoid send-on-closed-channel panics.
- Improved Redis rate limiter concurrency safety and circuit-breaker stats.
- Fixed AI HTTP response body reading so request contexts are cancelled after the body is read, not before.
- Updated Dockerfile module caching and deployment/env docs.

No secret values are included in this project. Use `.env.example` and rotate any keys/passwords that were previously pasted into chat.
