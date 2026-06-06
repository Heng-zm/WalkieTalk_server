# WalkieTalk Go

Go rewrite of the WalkieTalk FastAPI + Socket.IO server.

This version keeps the same core behavior but replaces Socket.IO with a native WebSocket endpoint:

```txt
/ws
```

WebSocket messages use this shape:

```json
{"event":"join_room","data":{"room":"ABC123","name":"kimheng"}}
```

## Features

- REST endpoints: `/`, `/health`, `/ready`, `/stats`, `/config/mapbox`, `/ai/chat`, `/zones`
- Native WebSocket realtime server
- Room join/leave/name update
- Push-to-talk voice relay
- Live voice chunks
- AI chat relay to your existing `AI_ASSISTANT_URL` or `AI_CHAT_URL`
- WebRTC screen-share signaling only
- SDP sanitizer for malformed browser SDP
- Local rate limit fallback
- Optional Redis distributed rate limiting
- Supabase REST integration for `geo_zones`
- Production CORS and optional `PUBLIC_API_KEY` protection

## Important migration note

Your Python server used Socket.IO. This Go version uses native WebSocket because Go Socket.IO libraries are less stable than native WebSocket.

The included `web/index.html` has already been updated for the Go backend. It uses a small `WTNativeSocket` wrapper so the existing UI code can still call:

```js
socket.emit("join_room", { room, name });
socket.on("voice_message", handler);
```

Internally the wrapper sends native WebSocket messages like:

```json
{"event":"join_room","data":{"room":"ABC123","name":"kimheng"}}
```

Optional frontend runtime config:

```html
<script>
  window.WT_SERVER_URL = "https://your-go-backend.onrender.com";
  // Only for private/admin deployments. Do not expose this on public websites.
  window.WT_PUBLIC_API_KEY = "your-public-api-key";
</script>
```

You can also store these in browser localStorage keys:

```txt
wt_server_url
wt_public_api_key
```

## Run locally

```bash
cp .env.example .env
# edit .env
export $(grep -v '^#' .env | xargs)
go run ./cmd/server
```

Open:

```txt
http://localhost:3000/health
http://localhost:3000/web/index.html
```

For local static test, you can also open `web/index.html` directly in your browser.

## Build

```bash
go mod tidy
go build -o walkietalk-go ./cmd/server
./walkietalk-go
```

## Docker

```bash
docker build -t walkietalk-go .
docker run --env-file .env -p 3000:3000 walkietalk-go
```

## Environment variables

Use `.env.example` as the template.

Clean old env values before deploying:

```env
MAPBOX_ACCESS_TOKEN=pk_your_token_without_quotes_or_newline
REDIS_ENABLED=true
```

Do not use:

```env
MAPBOX_ACCESS_TOKEN="pk...\n"
REDIS_ENABLED="True\n"
```

## Security

Set `PUBLIC_API_KEY` in production. When set, the server protects:

- `/stats`
- `POST /zones`
- `DELETE /zones/{id}`
- `/ai/chat`

Send it from trusted admin clients only:

```bash
curl -H "X-Api-Key: your-key" https://your-server/stats
```

Rotate any API key or Redis password that was pasted into a chat or public log.

## WebSocket events

Client -> server:

- `join_room`, alias: `join`
- `leave_room`, alias: `leave`
- `update_name`
- `voice_message`
- `voice_chunk`
- `voice_stream_end`
- `ai_chat_message`
- `screen_share_start`
- `screen_share_stop`
- `screen_share_state`
- `screen_viewer_ready`
- `screen_offer`
- `screen_answer`
- `screen_ice_candidate`
- `quality_pong`

Server -> client:

- `connected`
- `room_state`
- `peer_joined`
- `peer_left`
- `peer_name_updated`
- `voice_message`
- `voice_chunk`
- `voice_stream_end`
- `ai_chat_typing`
- `ai_chat_response`
- `ai_chat_error`
- `quality_ping`
- `quality_update`
- `screen_share_started`
- `screen_share_stopped`
- `screen_share_state`
- `screen_viewer_ready`
- `screen_offer`
- `screen_answer`
- `screen_ice_candidate`
- `screen_share_error`
- `zone_updated`
- `zone_deleted`

## Supabase table example

```sql
create table if not exists public.geo_zones (
  id text primary key,
  device_id text not null,
  name text not null default 'Zone',
  color text not null default '#007aff',
  lat double precision,
  lng double precision,
  radius_m double precision,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists geo_zones_device_id_idx on public.geo_zones(device_id);
```

## Production notes

- Deploy as one instance unless you add WebSocket pub/sub fanout.
- Redis is used for rate limiting, not cross-instance WebSocket room fanout.
- Use sticky sessions if you scale multiple instances.
- Restrict Mapbox `pk` token by allowed domains in Mapbox dashboard.
