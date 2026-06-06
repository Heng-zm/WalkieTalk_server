# WalkieTalk Go

Go backend + single-page web client for WalkieTalk push-to-talk voice channels.

This version uses a native WebSocket endpoint instead of Socket.IO:

```txt
/ws
```

WebSocket messages use this shape:

```json
{"event":"join_room","data":{"room":"ABC123","name":"kimheng"}}
```

## Features

- REST endpoints: `/`, `/health`, `/ready`, `/stats`, `/channels`, `/config/mapbox`, `/zones`
- Native WebSocket realtime server
- Room/channel join, leave, rename, and live member count
- Channel list page/sheet with `ចំនួនមនុស្សក្នុង channel`
- Empty user-created channels expire after 15 minutes by default
- Push-to-talk voice relay
- Live voice chunks
- Local rate-limit fallback
- Optional Redis distributed rate limiting
- Supabase REST integration for `geo_zones`
- Production CORS and optional `PUBLIC_API_KEY` protection
- Clean mobile-first UI

Removed in this build:

- AI assistant UI and `/ai/chat` functionality
- Screen-share UI and WebRTC signaling events

## Channel behavior

A channel is created when the first user joins it. The `/channels` endpoint and channel sheet show:

- channel name
- current user count
- current members
- empty expiration time

When the last user leaves a channel, it stays visible for `CHANNEL_EMPTY_TTL_SECS`. Default:

```env
CHANNEL_EMPTY_TTL_SECS=900
```

That is 15 minutes. After that, the backend removes the empty channel from the in-memory channel list.

## Important migration note

Your Python server used Socket.IO. This Go version uses native WebSocket because Go Socket.IO libraries are less stable than native WebSocket.

The included `web/index.html` uses a small `WTNativeSocket` wrapper so UI code can still call:

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

## Build

```bash
go mod tidy
go test ./...
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
CHANNEL_EMPTY_TTL_SECS=900
```

Do not use quoted values with pasted newlines:

```env
MAPBOX_ACCESS_TOKEN="pk...\n"
REDIS_ENABLED="True\n"
```

## Security

Set `PUBLIC_API_KEY` only when you need private/admin protection. When set, the server protects `/stats`. Zone write protection is separately controlled by:

```env
ZONE_WRITE_REQUIRES_API_KEY=false
```

For a public user app, keep zone writes device-scoped and leave `ZONE_WRITE_REQUIRES_API_KEY=false`. For a private/admin deployment, set it to `true` and send the key as `X-Api-Key`.

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
- `channels_list`, alias: `channels_refresh`
- `quality_pong`

Server -> client:

- `connected`
- `room_state`
- `peer_joined`
- `peer_left`
- `peer_name_updated`
- `channels_state`
- `channels_expired`
- `voice_message`
- `voice_chunk`
- `voice_stream_end`
- `quality_ping`
- `quality_update`
- `zone_updated`
- `zone_deleted`

Legacy AI and screen-share events return `FEATURE_DISABLED`.

## Supabase table example

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
```

A full migration is included at:

```txt
sql/geo_zones.sql
```

## Production notes

- Deploy as one instance unless you add WebSocket pub/sub fanout.
- The in-memory channel list is per running backend instance.
- Redis is used for rate limiting, not cross-instance WebSocket room fanout.
- Use sticky sessions if you scale multiple instances.
- Restrict Mapbox `pk` token by allowed domains in Mapbox dashboard.
