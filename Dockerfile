FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy module files first for Docker layer caching. go.sum is optional during
# early local development, but should be committed for reproducible builds.
COPY go.mod go.sum* ./
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
