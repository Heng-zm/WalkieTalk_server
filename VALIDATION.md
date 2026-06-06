# Validation notes

Performed in this ChatGPT container for the split deployment package:

- Backend Go formatting:

```bash
cd backend
gofmt -w $(find . -name '*.go')
```

Result: passed.

- Backend compile/test check using local stubs for external modules, because this container cannot reach `proxy.golang.org`:

```bash
cd backend
go mod edit -replace=github.com/gorilla/websocket=/mnt/data/go_stubs/websocket
go mod edit -replace=github.com/redis/go-redis/v9=/mnt/data/go_stubs/redis
go test ./...
```

Result: passed for all packages.

The real project `go.mod` was restored after the local-stub validation.

- Frontend inline JavaScript syntax check:

```bash
node --check /mnt/data/frontend-inline-split.js
```

Result: passed.

Not completed in this container:

- Real `go mod download`, `go test ./...`, and `go build ./cmd/server` with internet access. This container cannot download from `proxy.golang.org`.

Run this in Render or your machine with internet:

```bash
cd backend
go mod download
go mod tidy
go test ./...
go build ./cmd/server
```

Split deployment checks:

- Backend no longer registers `/web/` static hosting.
- Dockerfile no longer copies the `web` folder.
- Frontend is in `frontend/` for Vercel.
- Frontend does not use `web/env.js`, `WT_ENV`, or frontend secret keys.
- Frontend calls the Render backend through `DEFAULT_BACKEND_API_URL` in `frontend/index.html`.
