# Validation notes

Performed in this ChatGPT container after the អាយកូម channel-security/UI update:

- `gofmt -w $(find . -name '*.go')` completed successfully.
- Frontend inline JavaScript was extracted from `frontend/index.html` and checked with:

```bash
node --check /mnt/data/frontend-inline-v2.js
```

Result: JavaScript syntax check passed.

- Go package compile/test was checked with local stubs for external modules because this container cannot reach `proxy.golang.org`:

```bash
go mod edit -replace=github.com/gorilla/websocket=/mnt/data/go_stubs/websocket
go mod edit -replace=github.com/redis/go-redis/v9=/mnt/data/go_stubs/redis
go test ./...
```

Result: all packages compiled and `go test ./...` passed with no test files.

Not completed in this container:

- A real internet-backed `go mod download`, `go test ./...`, and `go build ./cmd/server` could not be performed because DNS access to `proxy.golang.org` is blocked in this container.

Run this on Render or any machine with internet access:

```bash
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Validated update scope:

- Reconnect upgrade with backend wake call and last-channel auto rejoin.
- Better channel expiration with countdown metadata.
- Private channel invite-code generation and PIN verification.
- New អាយកូម app/PWA icon and Khmer branding.
- Khmer-friendly error messages.
- Channel search and public/private channel UI.
