package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string
	Env  string

	CORSOrigins  []string
	PublicAPIKey string

	SupabaseURL string
	SupabaseKey string

	RedisEnabled          bool
	RedisURL              string
	RedisCircuitOpenSecs  time.Duration
	RedisFailureThreshold int

	MapboxAccessToken            string
	MapboxStandardStyle          string
	MapboxStandardSatelliteStyle string
	PublicConfigCacheSecs        int

	AIAssistantURL    string
	AIChatURL         string
	AIAssistantAPIKey string
	AITimeout         time.Duration
	AIChatTimeout     time.Duration
	AIRetryAttempts   int
	AIRetryBaseDelay  time.Duration

	MaxRoomSize             int
	MaxNameLen              int
	MaxRoomLen              int
	MaxAudioBytes           int
	MaxAudioBase64Chars     int
	MaxDuration             time.Duration
	MaxMsgRate              int
	MsgRateWindow           time.Duration
	MaxChunkBytes           int
	MaxChunkRate            int
	MaxAITextLen            int
	MaxAIHistory            int
	MaxAIChatRate           int
	MaxScreenSignalRate     int
	MaxScreenSDPChars       int
	MaxScreenICEChars       int
	ScreenStateTTL          time.Duration
	ZoneTTL                 time.Duration
	ChannelEmptyTTL         time.Duration
	ZoneWriteRequiresAPIKey bool
	MaxZoneReadRate         int
	MaxZoneWriteRate        int

	KeepAliveEnabled  bool
	KeepAliveURL      string
	KeepAlivePath     string
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	KeepAliveToken    string
	ReadinessCacheTTL time.Duration
	InstanceID        string
}

func Load() Config {
	maxAudioBytes := envInt("MAX_AUDIO_BYTES", 8_000_000, 256_000, 24_000_000)
	return Config{
		Port:                         env("PORT", "3000"),
		Env:                          env("ENV", "development"),
		CORSOrigins:                  envList("CORS_ORIGINS", "*"),
		PublicAPIKey:                 env("PUBLIC_API_KEY", ""),
		SupabaseURL:                  strings.TrimRight(env("SUPABASE_URL", ""), "/"),
		SupabaseKey:                  env("SUPABASE_KEY", ""),
		RedisEnabled:                 envBool("REDIS_ENABLED", env("REDIS_URL", "") != ""),
		RedisURL:                     env("REDIS_URL", ""),
		RedisCircuitOpenSecs:         time.Duration(envFloat("REDIS_CIRCUIT_OPEN_SECS", 15, 1, 300)) * time.Second,
		RedisFailureThreshold:        envInt("REDIS_FAILURE_THRESHOLD", 3, 1, 20),
		MapboxAccessToken:            firstNonEmpty(env("MAPBOX_ACCESS_TOKEN", ""), env("MAPBOX_TOKEN", "")),
		MapboxStandardStyle:          env("MAPBOX_STANDARD_STYLE", "mapbox://styles/mapbox/standard"),
		MapboxStandardSatelliteStyle: env("MAPBOX_STANDARD_SATELLITE_STYLE", "mapbox://styles/mapbox/standard-satellite"),
		PublicConfigCacheSecs:        envInt("PUBLIC_CONFIG_CACHE_SECS", 300, 0, 86400),
		AIAssistantURL:               env("AI_ASSISTANT_URL", "https://bot-voice-sqnz.onrender.com/ai-assistant"),
		AIChatURL:                    env("AI_CHAT_URL", ""),
		AIAssistantAPIKey:            firstNonEmpty(env("AI_ASSISTANT_API_KEY", ""), env("AI_API_KEY", "")),
		AITimeout:                    time.Duration(envFloat("AI_TIMEOUT_SECS", 45, 5, 120)) * time.Second,
		AIChatTimeout:                time.Duration(envFloat("AI_CHAT_TIMEOUT_SECS", envFloat("AI_TIMEOUT_SECS", 45, 5, 120), 5, 120)) * time.Second,
		AIRetryAttempts:              envInt("AI_RETRY_ATTEMPTS", 2, 1, 5),
		AIRetryBaseDelay:             time.Duration(envFloat("AI_RETRY_BASE_DELAY", 0.45, 0.05, 5) * float64(time.Second)),
		MaxRoomSize:                  envInt("MAX_ROOM_SIZE", 20, 2, 200),
		MaxNameLen:                   32,
		MaxRoomLen:                   40,
		MaxAudioBytes:                maxAudioBytes,
		MaxAudioBase64Chars:          int(float64(maxAudioBytes)*1.38) + 2048,
		MaxDuration:                  time.Duration(envFloat("MAX_DURATION", 65, 1, 300)) * time.Second,
		MaxMsgRate:                   envInt("MAX_MSG_RATE", 4, 1, 60),
		MsgRateWindow:                time.Duration(envFloat("MSG_RATE_WINDOW", 10, 1, 60)) * time.Second,
		MaxChunkBytes:                envInt("MAX_CHUNK_BYTES", 220_000, 32_000, 1_500_000),
		MaxChunkRate:                 envInt("MAX_CHUNK_RATE", 40, 4, 120),
		MaxAITextLen:                 envInt("MAX_AI_TEXT_LEN", 2_000, 64, 8_000),
		MaxAIHistory:                 envInt("MAX_AI_HISTORY", 12, 0, 40),
		MaxAIChatRate:                envInt("MAX_AI_CHAT_RATE", 8, 1, 60),
		MaxScreenSignalRate:          envInt("MAX_SCREEN_SIGNAL_RATE", 50, 5, 240),
		MaxScreenSDPChars:            envInt("MAX_SCREEN_SDP_CHARS", 80_000, 4_000, 250_000),
		MaxScreenICEChars:            envInt("MAX_SCREEN_ICE_CHARS", 16_000, 1_000, 80_000),
		ScreenStateTTL:               time.Duration(envInt("SCREEN_STATE_TTL", 6*3600, 60, 24*3600)) * time.Second,
		ZoneTTL:                      time.Duration(envInt("ZONE_TTL_SECS", 5*3600, 300, 7*24*3600)) * time.Second,
		ChannelEmptyTTL:              time.Duration(envInt("CHANNEL_EMPTY_TTL_SECS", 15*60, 60, 24*3600)) * time.Second,
		ZoneWriteRequiresAPIKey:      envBool("ZONE_WRITE_REQUIRES_API_KEY", false),
		MaxZoneReadRate:              envInt("MAX_ZONE_READ_RATE", 60, 1, 600),
		MaxZoneWriteRate:             envInt("MAX_ZONE_WRITE_RATE", 20, 1, 120),
		KeepAliveEnabled:             envBool("KEEP_ALIVE_ENABLED", true),
		KeepAliveURL:                 strings.TrimRight(firstNonEmpty(env("KEEP_ALIVE_URL", ""), env("RENDER_EXTERNAL_URL", ""), env("SERVER_URL", "")), "/"),
		KeepAlivePath:                cleanPath(env("KEEP_ALIVE_PATH", "/webhook/keepalive")),
		KeepAliveInterval:            time.Duration(envInt("KEEP_ALIVE_INTERVAL_SECS", 300, 60, 3600)) * time.Second,
		KeepAliveTimeout:             time.Duration(envFloat("KEEP_ALIVE_TIMEOUT_SECS", 8, 1, 60)) * time.Second,
		KeepAliveToken:               env("KEEP_ALIVE_TOKEN", ""),
		ReadinessCacheTTL:            time.Duration(envInt("READINESS_CACHE_SECS", 20, 0, 300)) * time.Second,
		InstanceID:                   "inst_" + randomHex(6),
	}
}

func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func env(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	v = strings.Trim(v, "\"")
	v = strings.ReplaceAll(v, `\n`, "")
	return strings.TrimSpace(v)
}

func envList(key, fallback string) []string {
	raw := env(key, fallback)
	if raw == "" || raw == "*" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(env(key, ""))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "y", "on", "enable", "enabled":
		return true
	case "0", "false", "no", "n", "off", "disable", "disabled":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback, minValue, maxValue int) int {
	v, err := strconv.Atoi(env(key, ""))
	if err != nil {
		v = fallback
	}
	if v < minValue {
		v = minValue
	}
	if v > maxValue {
		v = maxValue
	}
	return v
}

func envFloat(key string, fallback, minValue, maxValue float64) float64 {
	v, err := strconv.ParseFloat(env(key, ""), 64)
	if err != nil {
		v = fallback
	}
	if v < minValue {
		v = minValue
	}
	if v > maxValue {
		v = maxValue
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
