# Validation notes

Performed in this ChatGPT container:

- `gofmt -w $(find . -name '*.go')` completed successfully.
- `go test ./...` could not complete because this container cannot reach `proxy.golang.org` to download external Go modules:
  - `github.com/gorilla/websocket`
  - `github.com/redis/go-redis/v9`

On a normal machine or Render/Railway build environment with internet access, run:

```bash
go mod tidy
go test ./...
go build ./cmd/server
```

No secret values are included in this project. Use `.env.example` and rotate any keys/passwords that were previously pasted into chat.
