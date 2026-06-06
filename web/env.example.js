// Copy to web/env.js for local/static hosting.
// Frontend env must contain public values only. Do not put REDIS_URL or SUPABASE_KEY here.
window.WT_ENV = {
  API_BASE_URL: "https://walkietalk-server-4pmn.onrender.com",
  WS_URL: "",
  MAPBOX_CONFIG_URL: "https://walkietalk-server-4pmn.onrender.com/config/mapbox",
  DEFAULT_ROOM: "DEMO",
  // Only needed when ZONE_WRITE_REQUIRES_API_KEY=true or for protected admin endpoints.
  PUBLIC_API_KEY: "",
  ENABLE_DEBUG: false
};
