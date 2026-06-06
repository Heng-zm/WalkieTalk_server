package realtime

import (
	"context"
	"regexp"
	"strings"
	"time"

	"walkietalk-go/internal/util"
)

var (
	sdpControlRe  = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	sdpSSRCMsidRe = regexp.MustCompile(`^(a=ssrc:\d+)\s+msid:\s*([^\s]+)\s+([^\s]+).*$`)
)

func (h *Hub) handleScreenStart(ctx context.Context, sid string, data map[string]any) {
	if !h.rate.Check(ctx, sid+":signal", h.cfg.MaxScreenSignalRate, h.cfg.MsgRateWindow) {
		h.emitScreenError(sid, "RATE_LIMITED", "Screen sharing signaling too fast", nil)
		return
	}
	room, name := h.roomName(sid)
	if room == "" {
		h.emitScreenError(sid, "NOT_IN_ROOM", "Join a room before sharing your screen", nil)
		return
	}
	allowTakeover := anyBool(data["takeover"])
	h.mu.Lock()
	current := h.screens[room]
	if current != nil && current.SenderSID != sid && !allowTakeover {
		h.mu.Unlock()
		h.emitScreenError(sid, "SCREEN_BUSY", "Another user is already sharing", map[string]any{"screen_share": current})
		return
	}
	if current != nil && current.SenderSID != sid && allowTakeover {
		delete(h.screens, room)
	}
	state := &ScreenState{
		Room:       room,
		StreamID:   cleanStreamID(data["stream_id"]),
		SenderSID:  sid,
		SenderName: name,
		Kind:       cleanScreenKind(data["kind"]),
		Title:      util.CleanSmallText(anyString(data["title"]), 120),
		HasAudio:   anyBool(data["has_audio"]),
		StartedAt:  float64(time.Now().UnixNano()) / 1e9,
	}
	h.screens[room] = state
	h.mu.Unlock()
	if current != nil && current.SenderSID != sid && allowTakeover {
		h.Broadcast(room, "screen_share_stopped", map[string]any{"room": room, "stream_id": current.StreamID, "sender_sid": current.SenderSID, "reason": "takeover"}, "")
	}
	h.Broadcast(room, "screen_share_started", state, "")
	h.Broadcast(room, "screen_share_state", map[string]any{"screen_share": state}, "")
}

func (h *Hub) handleScreenStop(sid string, data map[string]any) {
	room, _ := h.roomName(sid)
	if room == "" {
		return
	}
	h.mu.Lock()
	current := h.screens[room]
	if current == nil {
		h.mu.Unlock()
		h.Send(sid, "screen_share_state", map[string]any{"screen_share": nil})
		return
	}
	if current.SenderSID != sid {
		h.mu.Unlock()
		h.emitScreenError(sid, "NOT_OWNER", "Only the active sharer can stop this screen share", nil)
		return
	}
	delete(h.screens, room)
	h.mu.Unlock()
	reason := util.CleanSmallText(anyString(data["reason"]), 40)
	if reason == "" {
		reason = "stopped"
	}
	h.Broadcast(room, "screen_share_stopped", map[string]any{"room": room, "stream_id": current.StreamID, "sender_sid": sid, "reason": reason}, "")
	h.Broadcast(room, "screen_share_state", map[string]any{"screen_share": nil}, "")
}

func (h *Hub) handleScreenState(sid string) {
	room, _ := h.roomName(sid)
	if room == "" {
		h.Send(sid, "screen_share_state", map[string]any{"screen_share": nil})
		return
	}
	h.mu.RLock()
	state := h.screens[room]
	h.mu.RUnlock()
	h.Send(sid, "screen_share_state", map[string]any{"screen_share": state})
}

func (h *Hub) handleScreenStateForRoom(room string) {
	h.mu.RLock()
	state := h.screens[room]
	h.mu.RUnlock()
	h.Broadcast(room, "screen_share_state", map[string]any{"screen_share": state}, "")
}

func (h *Hub) handleViewerReady(ctx context.Context, sid string, data map[string]any) {
	if !h.checkSignal(ctx, sid) {
		return
	}
	room, name := h.roomName(sid)
	if room == "" {
		return
	}
	h.mu.RLock()
	state := h.screens[room]
	h.mu.RUnlock()
	if state == nil {
		h.emitScreenError(sid, "NO_SCREEN", "No active screen share in this room", nil)
		return
	}
	if state.SenderSID == sid {
		return
	}
	h.Send(state.SenderSID, "screen_viewer_ready", map[string]any{"viewer_sid": sid, "viewer_name": name, "stream_id": state.StreamID})
}

func (h *Hub) handleScreenOffer(ctx context.Context, sid string, data map[string]any) {
	if !h.checkSignal(ctx, sid) {
		return
	}
	target := util.CleanSmallText(anyString(data["target_sid"]), 128)
	sdp := sdpFromData(data, "sdp")
	if target == "" || sdp == "" {
		h.emitScreenError(sid, "BAD_OFFER", "Missing target_sid or valid SDP offer", nil)
		return
	}
	room, name := h.roomName(sid)
	if !h.sidInRoom(target, room) {
		h.emitScreenError(sid, "TARGET_NOT_FOUND", "Viewer is not in this room", nil)
		return
	}
	h.Send(target, "screen_offer", map[string]any{"sender_sid": sid, "sender_name": name, "sdp": sdp, "type": cleanWebRTCType(data["type"], "offer"), "stream_id": trim(anyString(data["stream_id"]), 48)})
}

func (h *Hub) handleScreenAnswer(ctx context.Context, sid string, data map[string]any) {
	if !h.checkSignal(ctx, sid) {
		return
	}
	target := util.CleanSmallText(anyString(data["target_sid"]), 128)
	sdp := sdpFromData(data, "sdp")
	if target == "" || sdp == "" {
		h.emitScreenError(sid, "BAD_ANSWER", "Missing target_sid or valid SDP answer", nil)
		return
	}
	room, name := h.roomName(sid)
	if !h.sidInRoom(target, room) {
		h.emitScreenError(sid, "TARGET_NOT_FOUND", "Sharer is not in this room", nil)
		return
	}
	h.Send(target, "screen_answer", map[string]any{"sender_sid": sid, "sender_name": name, "sdp": sdp, "type": cleanWebRTCType(data["type"], "answer"), "stream_id": trim(anyString(data["stream_id"]), 48)})
}

func (h *Hub) handleScreenICE(ctx context.Context, sid string, data map[string]any) {
	if !h.checkSignal(ctx, sid) {
		return
	}
	target := util.CleanSmallText(anyString(data["target_sid"]), 128)
	candidate := cleanICECandidate(data["candidate"], h.cfg.MaxScreenICEChars)
	if target == "" || candidate == nil {
		return
	}
	room, name := h.roomName(sid)
	if !h.sidInRoom(target, room) {
		return
	}
	h.Send(target, "screen_ice_candidate", map[string]any{"sender_sid": sid, "sender_name": name, "candidate": candidate, "stream_id": trim(anyString(data["stream_id"]), 48)})
}

func (h *Hub) checkSignal(ctx context.Context, sid string) bool {
	if !h.rate.Check(ctx, sid+":signal", h.cfg.MaxScreenSignalRate, h.cfg.MsgRateWindow) {
		h.emitScreenError(sid, "RATE_LIMITED", "Screen sharing signaling too fast", nil)
		return false
	}
	return true
}

func (h *Hub) emitScreenError(sid, code, msg string, extra map[string]any) {
	payload := map[string]any{"code": code, "msg": msg}
	for k, v := range extra {
		payload[k] = v
	}
	h.Send(sid, "screen_share_error", payload)
}

func (h *Hub) stopScreenShareForSID(sid, reason string) {
	var room string
	var state *ScreenState
	h.mu.Lock()
	for r, s := range h.screens {
		if s.SenderSID == sid {
			room = r
			state = s
			delete(h.screens, r)
			break
		}
	}
	h.mu.Unlock()
	if room != "" && state != nil {
		h.Broadcast(room, "screen_share_stopped", map[string]any{"room": room, "stream_id": state.StreamID, "sender_sid": sid, "reason": reason}, "")
		h.Broadcast(room, "screen_share_state", map[string]any{"screen_share": nil}, "")
	}
}

func (h *Hub) sidInRoom(sid, room string) bool {
	if sid == "" || room == "" {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rooms[room] != nil && h.rooms[room][sid]
}

func cleanStreamID(v any) string {
	id := util.CleanSmallText(anyString(v), 48)
	if id == "" {
		return util.RandomID("screen_")
	}
	return id
}

func cleanScreenKind(v any) string {
	s := strings.ToLower(strings.TrimSpace(anyString(v)))
	if s == "screen" || s == "window" || s == "tab" {
		return s
	}
	return "screen"
}

func cleanWebRTCType(v any, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(anyString(v)))
	if s == "offer" || s == "answer" {
		return s
	}
	return fallback
}

func anyBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	default:
		return false
	}
}

func sdpFromData(data map[string]any, keys ...string) string {
	for _, k := range append(keys, "description", "desc", "offer", "answer") {
		if s := cleanSDP(data[k]); s != "" {
			return s
		}
	}
	return ""
}

func cleanSDP(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s := cleanSDP(m["sdp"]); s != "" {
			return s
		}
		return cleanSDP(m["value"])
	}
	raw := strings.TrimSpace(anyString(v))
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\r\n`, "\n")
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	raw = strings.ReplaceAll(raw, `\r`, "\n")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	allowed := "vosiuepcbtrzkam"
	lines := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(sdpControlRe.ReplaceAllString(line, ""))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "a=ssrc:") && strings.Contains(line, " msid") {
			if m := sdpSSRCMsidRe.FindStringSubmatch(line); len(m) == 4 {
				line = m[1] + " msid:" + m[2] + " " + m[3]
			}
		}
		if len(line) < 2 || line[1] != '=' || !strings.ContainsRune(allowed, rune(line[0])) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	if lines[0] != "v=0" {
		found := -1
		for i, line := range lines {
			if line == "v=0" {
				found = i
				break
			}
		}
		if found < 0 {
			return ""
		}
		lines = lines[found:]
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func cleanICECandidate(v any, max int) any {
	if m, ok := v.(map[string]any); ok {
		candidate := strings.TrimSpace(anyString(m["candidate"]))
		candidate = strings.TrimPrefix(candidate, "a=")
		if candidate == "" || len(candidate) > max {
			return nil
		}
		out := map[string]any{"candidate": candidate}
		if s := util.CleanSmallText(anyString(m["sdpMid"]), 64); s != "" {
			out["sdpMid"] = s
		}
		if idx := anyFloat(m["sdpMLineIndex"]); idx >= 0 {
			out["sdpMLineIndex"] = int(idx)
		}
		if s := util.CleanSmallText(anyString(m["usernameFragment"]), 128); s != "" {
			out["usernameFragment"] = s
		}
		return out
	}
	candidate := strings.TrimSpace(anyString(v))
	candidate = strings.TrimPrefix(candidate, "a=")
	if candidate == "" || len(candidate) > max {
		return nil
	}
	return candidate
}
