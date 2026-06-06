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
- Production CORS, optional `PUBLIC_API_KEY` protection, and device-scoped zone sync

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

Optional frontend runtime config is loaded from `web/env.js`. Keep it public-safe:

```js
window.WT_ENV = {
  API_BASE_URL: "https://your-go-backend.onrender.com",
  WS_URL: "",
  MAPBOX_CONFIG_URL: "https://your-go-backend.onrender.com/config/mapbox",
  PUBLIC_API_KEY: ""
};
```

When the frontend is served from the same Go backend at `/web/index.html`, it uses the same origin automatically. For static hosting, set `WT_ENV.API_BASE_URL` in `web/env.js` or store these browser localStorage keys:

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

`PUBLIC_API_KEY` protects admin/private HTTP endpoints:

- `/stats`
- `/ai/chat`

Zone writes are device-scoped and public by default so the normal browser app can save zones without causing `401 Unauthorized`. To protect zone writes too, set:

```env
ZONE_WRITE_REQUIRES_API_KEY=true
PUBLIC_API_KEY=your-private-key
```

Then send the key from trusted clients only:

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

The full safe migration is included in `sql/geo_zones.sql`. You can paste it into Supabase SQL Editor. Core schema:

```sql
create table if not exists public.geo_zones (
  id text primary key,
  device_id text not null,
  name text not null default 'Zone',
  channel text not null default 'ZONE',
  color text not null default '#007aff',
  lat double precision not null,
  lng double precision not null,
  radius_m double precision not null default 300,
  auto_join boolean not null default true,
  created_by text,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists geo_zones_device_id_idx on public.geo_zones(device_id);
create index if not exists geo_zones_expires_at_idx on public.geo_zones(expires_at);

create or replace function public.set_geo_zones_updated_at()
returns trigger language plpgsql as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

drop trigger if exists trg_geo_zones_updated_at on public.geo_zones;
create trigger trg_geo_zones_updated_at
before update on public.geo_zones
for each row execute function public.set_geo_zones_updated_at();
```

## Zone sync notes

- `GET /zones` should include `device_id` as a query param or `X-Device-Id` header. Missing `device_id` now returns an empty zone list instead of hard-failing with `400`.
- `POST /zones` validates `lat`, `lng`, and `radius_m`. It tries modern and legacy Supabase column variants so older tables do not fail immediately on unknown columns.
- The frontend sends both `device_id` query/header and `device_id` in the JSON body.

## Production notes

- Deploy as one instance unless you add WebSocket pub/sub fanout.
- Redis is used for rate limiting, not cross-instance WebSocket room fanout.
- Use sticky sessions if you scale multiple instances.
- Restrict Mapbox `pk` token by allowed domains in Mapbox dashboard.
