// Runtime frontend config. Keep this file public-safe.
// Edit values only when hosting the frontend separately from the Go backend.
window.WT_ENV = Object.assign({
  API_BASE_URL: "",
  WS_URL: "",
  MAPBOX_CONFIG_URL: "",
  DEFAULT_ROOM: "DEMO",
  PUBLIC_API_KEY: "",
  ENABLE_DEBUG: false
}, window.WT_ENV || {});
