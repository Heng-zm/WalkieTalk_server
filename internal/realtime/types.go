package realtime

import (
	"encoding/json"
	"time"
)

type Envelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type User struct {
	SID      string    `json:"sid"`
	Name     string    `json:"name"`
	Room     string    `json:"room"`
	JoinedAt time.Time `json:"joined_at"`
}

type RoomMember struct {
	SID  string `json:"sid"`
	Name string `json:"name"`
}

type ScreenState struct {
	Room       string  `json:"room"`
	StreamID   string  `json:"stream_id"`
	SenderSID  string  `json:"sender_sid"`
	SenderName string  `json:"sender_name"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	HasAudio   bool    `json:"has_audio"`
	StartedAt  float64 `json:"started_at"`
}

type QualityState struct {
	Pending map[string]time.Time
	RTTs    []float64
	Cycles  []bool
}
