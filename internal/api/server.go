package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"walkietalk-go/internal/ai"
	"walkietalk-go/internal/config"
	"walkietalk-go/internal/realtime"
	"walkietalk-go/internal/store"
	"walkietalk-go/internal/util"
)

type Server struct {
	cfg      config.Config
	hub      *realtime.Hub
	ai       *ai.Client
	rate     *store.RateStore
	log      *log.Logger
	start    time.Time
	client   *http.Client
	upgrader websocket.Upgrader
}

func NewServer(cfg config.Config, hub *realtime.Hub, aiClient *ai.Client, rate *store.RateStore, logger *log.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		hub:    hub,
		ai:     aiClient,
		rate:   rate,
		log:    logger,
		start:  time.Now(),
		client: &http.Client{Timeout: 20 * time.Second},
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return s.originAllowed(r.Header.Get("Origin")) },
	}
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/config/mapbox", s.handleMapboxConfig)
	mux.HandleFunc("/ai/chat", s.handleAIChat)
	mux.HandleFunc("/zones", s.handleZones)
	mux.HandleFunc("/zones/", s.handleZoneByID)
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))
	mux.HandleFunc("/socket.io", s.handleSocketIOMigrationNotice)
	return s.cors(mux)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"name":           "WalkieTalk Go",
		"status":         "ok",
		"health":         "/health",
		"ready":          "/ready",
		"stats":          "/stats",
		"zones":          "/zones",
		"ai_chat":        "/ai/chat",
		"mapbox_config":  "/config/mapbox",
		"websocket_path": "/ws",
		"socketio_path":  "replaced by /ws native WebSocket",
		"features": map[string]any{
			"voice_relay":            true,
			"live_voice_chunks":      true,
			"ai_chat":                true,
			"geo_zones":              true,
			"screen_share_signaling": true,
			"sdp_sanitizer":          true,
			"redis_rate_fallback":    true,
			"runtime_stats":          true,
			"mapbox_env_config":      s.cfg.MapboxAccessToken != "",
		},
		"events": []string{"join_room", "leave_room", "update_name", "voice_message", "voice_chunk", "voice_stream_end", "ai_chat_message", "screen_share_start", "screen_share_stop", "screen_share_state", "screen_viewer_ready", "screen_offer", "screen_answer", "screen_ice_candidate", "quality_pong"},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":              "ok",
		"instance":            s.cfg.InstanceID,
		"connections":         s.hub.Stats()["local_users"],
		"rooms_local":         s.hub.RoomsSnapshot(),
		"screen_shares_local": s.hub.ScreensSnapshot(),
		"redis":               s.rate.RedisOK(r.Context()),
		"supabase_configured": s.cfg.SupabaseURL != "" && s.cfg.SupabaseKey != "",
		"ai_configured":       s.cfg.AIChatURL != "" || s.cfg.AIAssistantURL != "",
		"ai_key_configured":   s.cfg.AIAssistantAPIKey != "",
		"mapbox_configured":   s.cfg.MapboxAccessToken != "",
		"uptime_s":            int(time.Since(s.start).Seconds()),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	redisOK := any(nil)
	if s.cfg.RedisEnabled && s.cfg.RedisURL != "" {
		redisOK = s.rate.RedisOK(r.Context())
	}
	supabaseOK := any(nil)
	if s.cfg.SupabaseURL != "" && s.cfg.SupabaseKey != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		resp, err := s.supabaseRequest(ctx, http.MethodGet, "/rest/v1/geo_zones?limit=1&select=id", nil, nil)
		supabaseOK = err == nil && resp >= 200 && resp < 300
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "instance": s.cfg.InstanceID, "redis": redisOK, "supabase": supabaseOK, "mapbox": s.cfg.MapboxAccessToken != "", "websocket": true, "uptime_s": int(time.Since(s.start).Seconds())})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		s.writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	stats := s.hub.Stats()
	stats["uptime_s"] = int(time.Since(s.start).Seconds())
	stats["redis"] = s.rate.Stats()
	stats["supabase_enabled"] = s.cfg.SupabaseURL != "" && s.cfg.SupabaseKey != ""
	stats["mapbox_enabled"] = s.cfg.MapboxAccessToken != ""
	s.writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleMapboxConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MapboxAccessToken == "" {
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "MAPBOX_ACCESS_TOKEN is not configured on backend server ENV"})
		return
	}
	if strings.HasPrefix(s.cfg.MapboxAccessToken, "sk.") {
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "Invalid Mapbox token type for browser. Use a public pk token, not a secret sk token."})
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", s.cfg.PublicConfigCacheSecs))
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": s.cfg.MapboxAccessToken, "styles": map[string]string{"standard": s.cfg.MapboxStandardStyle, "standard_satellite": s.cfg.MapboxStandardSatelliteStyle}, "default_style": "standard"})
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if s.cfg.PublicAPIKey != "" && !s.authorized(r) {
		s.writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	ipKey := "http_ai:" + clientIP(r)
	if !s.rate.Check(r.Context(), ipKey, s.cfg.MaxAIChatRate, s.cfg.MsgRateWindow) {
		s.writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "Slow down — too many AI messages"})
		return
	}
	body := map[string]any{}
	_ = json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&body)
	res := s.ai.BuildChat(r.Context(), body)
	status := http.StatusOK
	if !res.OK {
		status = http.StatusBadGateway
	}
	s.writeJSON(w, status, res)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sid := util.RandomID("sid_")
	client := &realtime.Client{SID: sid, Hub: s.hub, Conn: conn, Send: make(chan realtime.Envelope, 256)}
	s.hub.Register(client)
	// Do not use r.Context() here: after a WebSocket upgrade, ServeHTTP returns
	// and the request context can be cancelled while the socket is still alive.
	ctx := context.Background()
	go client.WritePump(ctx)
	go client.ReadPump(ctx)
}

func (s *Server) handleSocketIOMigrationNotice(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusGone, map[string]any{"ok": false, "error": "Socket.IO was replaced by native WebSocket. Use /ws and JSON {event,data}."})
}

func (s *Server) handleZones(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getZones(w, r)
	case http.MethodPost:
		if !s.authorized(r) {
			s.writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		s.postZone(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleZoneByID(w http.ResponseWriter, r *http.Request) {
	zoneID := strings.TrimPrefix(r.URL.Path, "/zones/")
	zoneID = util.CleanSmallText(zoneID, 128)
	if zoneID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing zone id"})
		return
	}
	if r.Method != http.MethodDelete {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		s.writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	deviceID := util.CleanDevice(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "device_id is required"})
		return
	}
	path := "/rest/v1/geo_zones?id=eq." + url.QueryEscape(zoneID) + "&device_id=eq." + url.QueryEscape(deviceID)
	status, body, err := s.supabaseRequestBody(r.Context(), http.MethodDelete, path, nil, map[string]string{"Prefer": "return=representation"})
	if err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if status < 200 || status >= 300 {
		s.writeJSON(w, status, map[string]any{"ok": false, "error": string(body)})
		return
	}
	var deleted []map[string]any
	_ = json.Unmarshal(body, &deleted)
	s.hub.BroadcastAll("zone_deleted", map[string]any{"id": zoneID, "device_id": deviceID})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
}

func (s *Server) getZones(w http.ResponseWriter, r *http.Request) {
	deviceID := util.CleanDevice(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "device_id is required"})
		return
	}
	query := url.Values{}
	query.Set("select", "*")
	query.Set("device_id", "eq."+deviceID)
	query.Set("order", "created_at.desc")
	path := "/rest/v1/geo_zones?" + query.Encode()
	status, body, err := s.supabaseRequestBody(r.Context(), http.MethodGet, path, nil, nil)
	if err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if status < 200 || status >= 300 {
		s.writeJSON(w, status, map[string]any{"ok": false, "error": string(body)})
		return
	}
	var zones []map[string]any
	_ = json.Unmarshal(body, &zones)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "zones": zones})
}

func (s *Server) postZone(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid JSON"})
		return
	}
	deviceID := util.CleanDevice(anyString(body["device_id"]))
	if deviceID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "device_id is required"})
		return
	}
	id := util.CleanSmallText(anyString(body["id"]), 128)
	if id == "" {
		id = util.RandomID("zone_")
	}
	zone := map[string]any{
		"id":         id,
		"device_id":  deviceID,
		"name":       util.CleanSmallText(anyString(body["name"]), 80),
		"color":      util.CleanColor(anyString(body["color"])),
		"lat":        body["lat"],
		"lng":        body["lng"],
		"radius_m":   body["radius_m"],
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if zone["name"] == "" {
		zone["name"] = "Zone"
	}
	if body["expires_at"] != nil && anyString(body["expires_at"]) != "" {
		zone["expires_at"] = anyString(body["expires_at"])
	} else {
		zone["expires_at"] = time.Now().UTC().Add(s.cfg.ZoneTTL).Format(time.RFC3339)
	}
	payload, _ := json.Marshal([]map[string]any{zone})
	path := "/rest/v1/geo_zones?on_conflict=id"
	status, respBody, err := s.supabaseRequestBody(r.Context(), http.MethodPost, path, payload, map[string]string{"Prefer": "resolution=merge-duplicates,return=representation"})
	if err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if status < 200 || status >= 300 {
		s.writeJSON(w, status, map[string]any{"ok": false, "error": string(respBody)})
		return
	}
	var saved []map[string]any
	_ = json.Unmarshal(respBody, &saved)
	s.hub.BroadcastAll("zone_updated", map[string]any{"zone": zone})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "zone": zone, "saved": saved})
}

func (s *Server) supabaseRequest(ctx context.Context, method, path string, body []byte, extra map[string]string) (int, error) {
	status, _, err := s.supabaseRequestBody(ctx, method, path, body, extra)
	return status, err
}

func (s *Server) supabaseRequestBody(ctx context.Context, method, path string, body []byte, extra map[string]string) (int, []byte, error) {
	if s.cfg.SupabaseURL == "" || s.cfg.SupabaseKey == "" {
		return 0, nil, fmt.Errorf("SUPABASE_URL/SUPABASE_KEY not configured")
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, s.cfg.SupabaseURL+path, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("apikey", s.cfg.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+s.cfg.SupabaseKey)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 350 * time.Millisecond)
				continue
			}
			return 0, nil, err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if retryable(resp.StatusCode) && attempt < 3 {
			lastErr = fmt.Errorf("Supabase HTTP %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt) * 350 * time.Millisecond)
			continue
		}
		return resp.StatusCode, data, nil
	}
	return 0, nil, lastErr
}

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.PublicAPIKey == "" {
		return true
	}
	key := r.Header.Get("X-Api-Key")
	if key == "" {
		key = r.URL.Query().Get("api_key")
	}
	return key == s.cfg.PublicAPIKey
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.originAllowed(origin) {
			if len(s.cfg.CORSOrigins) == 1 && s.cfg.CORSOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Api-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	if len(s.cfg.CORSOrigins) == 1 && s.cfg.CORSOrigins[0] == "*" {
		return true
	}
	for _, allowed := range s.cfg.CORSOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func anyString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	return r.RemoteAddr
}

func retryable(code int) bool {
	switch code {
	case 408, 409, 425, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
