-- Safe Supabase SQL for WalkieTalk geo_zones.
-- Run this in Supabase SQL Editor. It creates the table if needed and upgrades older versions.

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

alter table public.geo_zones add column if not exists device_id text;
alter table public.geo_zones add column if not exists name text default 'Zone';
alter table public.geo_zones add column if not exists channel text default 'ZONE';
alter table public.geo_zones add column if not exists color text default '#007aff';
alter table public.geo_zones add column if not exists lat double precision;
alter table public.geo_zones add column if not exists lng double precision;
alter table public.geo_zones add column if not exists radius_m double precision default 300;
alter table public.geo_zones add column if not exists auto_join boolean default true;
alter table public.geo_zones add column if not exists created_by text;
alter table public.geo_zones add column if not exists expires_at timestamptz;
alter table public.geo_zones add column if not exists created_at timestamptz default now();
alter table public.geo_zones add column if not exists updated_at timestamptz default now();

-- If an older table used radius instead of radius_m, copy the values once.
do $$
begin
  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'geo_zones' and column_name = 'radius'
  ) then
    execute 'update public.geo_zones set radius_m = coalesce(radius_m, radius) where radius_m is null';
  end if;
end $$;

update public.geo_zones set name = coalesce(nullif(name, ''), 'Zone') where name is null or name = '';
update public.geo_zones set channel = coalesce(nullif(channel, ''), 'ZONE') where channel is null or channel = '';
update public.geo_zones set color = coalesce(nullif(color, ''), '#007aff') where color is null or color = '';
update public.geo_zones set radius_m = 300 where radius_m is null;
update public.geo_zones set auto_join = true where auto_join is null;
update public.geo_zones set created_at = now() where created_at is null;
update public.geo_zones set updated_at = now() where updated_at is null;

alter table public.geo_zones alter column device_id set not null;
alter table public.geo_zones alter column name set not null;
alter table public.geo_zones alter column channel set not null;
alter table public.geo_zones alter column color set not null;
alter table public.geo_zones alter column lat set not null;
alter table public.geo_zones alter column lng set not null;
alter table public.geo_zones alter column radius_m set not null;
alter table public.geo_zones alter column auto_join set not null;
alter table public.geo_zones alter column created_at set not null;
alter table public.geo_zones alter column updated_at set not null;

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

-- Recommended when using SUPABASE_KEY as service role from the backend:
-- keep RLS on if you have policies, but service role bypasses RLS.
-- alter table public.geo_zones enable row level security;
