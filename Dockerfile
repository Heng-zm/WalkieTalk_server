FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy go.mod first for Docker layer caching. go.sum is generated/updated after
# copying the source so Render builds do not fail when go.sum is missing locally.
COPY go.mod ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/walkietalk-go ./cmd/server

FROM alpine:3.20
RUN adduser -D -g '' appuser
USER appuser
WORKDIR /app
COPY --from=build /out/walkietalk-go /app/walkietalk-go
COPY web /app/web
ENV PORT=3000
EXPOSE 3000
CMD ["/app/walkietalk-go"]
