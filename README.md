# WalkieTalk Go Backend — Render

Go API + native WebSocket server for WalkieTalk.

This backend is designed to be deployed separately from the frontend:

```txt
backend/   -> Render
frontend/  -> Vercel
```

The backend does **not** serve the frontend. `/web/index.html` is intentionally removed for split hosting.

## Features

- Native WebSocket endpoint: `/ws`
- Voice relay and live voice chunks
- Channel list API with live user counts: `/channels`
- Empty user-created channels expire after 15 minutes by default
- Geo zones API: `/zones`
- Mapbox public config endpoint: `/config/mapbox`
- Redis/local rate limiting fallback
- Supabase REST integration for `geo_zones`
- AI assistant removed/disabled
- Screen-share signaling removed/disabled

## Render environment

```env
PORT=3000
ENV=production

# Set this to your Vercel domain in production.
# Testing: CORS_ORIGINS=*
CORS_ORIGINS=https://your-vercel-app.vercel.app,http://localhost:3000,http://localhost:5173

# Leave empty for public frontend.
PUBLIC_API_KEY=
ZONE_WRITE_REQUIRES_API_KEY=false

MAPBOX_ACCESS_TOKEN=pk_your_public_mapbox_token
MAPBOX_STANDARD_STYLE=mapbox://styles/mapbox/standard
MAPBOX_STANDARD_SATELLITE_STYLE=mapbox://styles/mapbox/standard-satellite
PUBLIC_CONFIG_CACHE_SECS=300

SUPABASE_URL=https://your-project.supabase.co
SUPABASE_KEY=your_backend_service_role_key

REDIS_ENABLED=true
REDIS_URL=redis://default:replace-with-password@your-redis-host:6379
REDIS_CIRCUIT_OPEN_SECS=15
REDIS_FAILURE_THRESHOLD=3

MAX_ROOM_SIZE=20
MAX_AUDIO_BYTES=8000000
MAX_DURATION=65
MAX_MSG_RATE=4
MSG_RATE_WINDOW=10
MAX_CHUNK_BYTES=220000
MAX_CHUNK_RATE=40
MAX_ZONE_READ_RATE=60
MAX_ZONE_WRITE_RATE=20
ZONE_TTL_SECS=18000
CHANNEL_EMPTY_TTL_SECS=900
```

Do not put backend secrets in Vercel/frontend:

```txt
SUPABASE_KEY / service_role
REDIS_URL
secret/admin keys
```

## Endpoints

```txt
GET  /
GET  /health
GET  /ready
GET  /stats
GET  /channels
GET  /config/mapbox
GET  /zones?device_id=...
POST /zones
DELETE /zones/{id}
GET  /ws
```

## Local run

```bash
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go run ./cmd/server
```

Open:

```txt
http://localhost:3000/health
```

## Build/test

```bash
go mod tidy
go test ./...
go build -o walkietalk-go ./cmd/server
```

## Supabase

Run `sql/geo_zones.sql` in Supabase SQL Editor.

If RLS is enabled, use the backend-only Supabase `service_role` key as `SUPABASE_KEY` on Render.
