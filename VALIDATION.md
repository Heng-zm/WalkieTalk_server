# Validation notes

Performed in this ChatGPT container after the Aikom channel-security/UI update:

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
- New Aikom app/PWA icon and English branding.
- English-friendly error messages.
- Channel search and public/private channel UI.


## English-only optimization validation

- Backend Go files formatted with `gofmt`.
- User-facing backend error strings converted to English.
- `go test ./...` passed using local websocket/redis stubs because this container cannot reach the public Go module proxy.


## ACK event fix

- Added backend support for `msg_delivered`, `message_delivered`, `msg_read`, `message_read`, `msg_seen`, `message_seen`, `msg_received`, and `message_received`.
- ACK events are validated and forwarded only when the target user is online in the same channel.
- Invalid/offline ACKs are ignored safely to avoid noisy `UNKNOWN_EVENT` errors.


## Tailwind ACK hotfix validation

- Merged message acknowledgement support into the backend event router.
- Ran `gofmt` on all Go files.
- Ran `go test ./...` with local Redis/WebSocket stubs because this container cannot reach `proxy.golang.org`; compile check passed.
