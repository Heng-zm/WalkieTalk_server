package realtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"walkietalk-go/internal/util"
)

var allowedMIME = map[string]bool{
	"audio/webm":             true,
	"audio/webm;codecs=opus": true,
	"audio/mp4":              true,
	"audio/ogg":              true,
	"audio/wav":              true,
}

func (h *Hub) HandleEvent(ctx context.Context, c *Client, env Envelope) {
	data := decodeMap(env.Data)
	switch strings.TrimSpace(env.Event) {
	case "join", "join_room":
		h.handleJoin(c.SID, data)
	case "leave", "leave_room", "leave_room_event":
		h.handleLeave(c.SID)
	case "update_name":
		h.handleUpdateName(c.SID, data)
	case "voice_message":
		h.handleVoiceMessage(ctx, c.SID, data)
	case "voice_chunk":
		h.handleVoiceChunk(ctx, c.SID, data)
	case "voice_stream_end":
		h.handleVoiceStreamEnd(c.SID, data)
	case "channels_list", "channels_refresh":
		h.SendChannelsState(c.SID)
	case "ai_chat_message":
		h.Send(c.SID, "error", map[string]any{"code": "FEATURE_DISABLED", "msg": "AI assistant was removed from this version"})
	case "quality_pong":
		h.handleQualityPong(c.SID, data)
	case "screen_share_start", "screen_share_stop", "screen_share_state", "screen_viewer_ready", "screen_offer", "screen_answer", "screen_ice_candidate":
		h.Send(c.SID, "error", map[string]any{"code": "FEATURE_DISABLED", "msg": "Screen sharing was removed from this version"})
	default:
		h.Send(c.SID, "error", map[string]any{"code": "UNKNOWN_EVENT", "msg": "Unknown event: " + env.Event})
	}
}

func (h *Hub) handleJoin(sid string, data map[string]any) {
	room := util.CleanRoom(anyString(data["room"]), h.cfg.MaxRoomLen)
	name := util.CleanName(anyString(data["name"]), sidPrefix(sid), h.cfg.MaxNameLen)
	if room == "" {
		h.Send(sid, "error", map[string]any{"code": "BAD_CHANNEL", "msg": "សូមបញ្ចូលឈ្មោះឆានែលត្រឹមត្រូវ"})
		return
	}

	now := time.Now()
	h.mu.Lock()
	state, errCode, errMsg := h.prepareChannelJoinLocked(room, sid, name, data, now)
	if errCode != "" {
		h.mu.Unlock()
		h.Send(sid, "error", map[string]any{"code": errCode, "msg": errMsg, "room": room})
		return
	}
	alreadyMember := h.isRoomMemberLocked(room, sid)
	if !alreadyMember && len(h.rooms[room]) >= h.cfg.MaxRoomSize {
		h.mu.Unlock()
		h.Send(sid, "error", map[string]any{"code": "ROOM_FULL", "msg": "ឆានែលពេញហើយ"})
		return
	}
	_ = state
	h.mu.Unlock()

	oldRoom, oldName := h.leaveNoBroadcast(sid)
	if oldRoom != "" && oldRoom != room {
		h.Broadcast(oldRoom, "peer_left", map[string]any{"sid": sid, "name": oldName}, sid)
	}

	h.mu.Lock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[string]bool)
	}
	if len(h.rooms[room]) >= h.cfg.MaxRoomSize {
		h.mu.Unlock()
		h.Send(sid, "error", map[string]any{"code": "ROOM_FULL", "msg": "ឆានែលពេញហើយ"})
		return
	}
	h.rooms[room][sid] = true
	h.users[sid] = &User{SID: sid, Name: name, Room: room, JoinedAt: now}
	h.touchChannelLocked(room, now)
	state = h.channels[room]
	members := h.membersLocked(room)
	userCount := len(members)
	visibility := "public"
	private := false
	owner := false
	inviteCode := ""
	hasPIN := false
	if state != nil {
		visibility = state.Visibility
		if visibility == "" {
			visibility = "public"
		}
		private = visibility == "private"
		hasPIN = state.HasPIN
		owner = state.OwnerSID == sid
		if owner && private {
			inviteCode = state.InviteCode
		}
	}
	h.mu.Unlock()

	h.Broadcast(room, "peer_joined", map[string]any{"sid": sid, "name": name}, sid)
	h.Send(sid, "room_state", map[string]any{
		"room":        room,
		"channel":     room,
		"members":     members,
		"user_count":  userCount,
		"visibility":  visibility,
		"private":     private,
		"owner":       owner,
		"invite_code": inviteCode,
		"has_pin":     hasPIN,
	})
	h.BroadcastChannelsState()
	h.log.Printf("join sid=%s name=%s room=%s n=%d visibility=%s", sid, name, room, userCount, visibility)
}

func (h *Hub) handleLeave(sid string) {
	room, name := h.leaveNoBroadcast(sid)
	if room != "" {
		h.Broadcast(room, "peer_left", map[string]any{"sid": sid, "name": name}, sid)
		h.BroadcastChannelsState()
	}
}

func (h *Hub) handleUpdateName(sid string, data map[string]any) {
	newName := util.CleanName(anyString(data["name"]), "", h.cfg.MaxNameLen)
	if newName == "" {
		return
	}
	h.mu.Lock()
	u := h.users[sid]
	oldName := sidPrefix(sid)
	room := ""
	if u != nil {
		oldName = u.Name
		u.Name = newName
		room = u.Room
	}
	h.mu.Unlock()
	if room != "" {
		h.Broadcast(room, "peer_name_updated", map[string]any{"sid": sid, "name": newName}, sid)
		h.BroadcastChannelsState()
	}
	h.log.Printf("rename %s -> %s", oldName, newName)
}

func (h *Hub) handleVoiceMessage(ctx context.Context, sid string, data map[string]any) {
	room, name := h.roomName(sid)
	if room == "" {
		return
	}
	audio := util.StripDataURLBase64(anyString(data["audio"]))
	if audio == "" {
		return
	}
	if len(audio) > h.cfg.MaxAudioBase64Chars {
		h.Send(sid, "error", map[string]any{"code": "MSG_TOO_LARGE", "msg": "Audio too large"})
		return
	}
	if !h.rate.Check(ctx, sid+":voice", h.cfg.MaxMsgRate, h.cfg.MsgRateWindow) {
		h.Send(sid, "error", map[string]any{"code": "RATE_LIMITED", "msg": "Sending too fast"})
		return
	}
	mime := anyString(data["mime"])
	if !allowedMIME[mime] {
		mime = "audio/webm"
	}
	duration := anyFloat(data["duration"])
	maxDuration := h.cfg.MaxDuration.Seconds()
	if duration > maxDuration {
		duration = maxDuration
	}
	payload := map[string]any{
		"audio":       audio,
		"mime":        mime,
		"duration":    round1(duration),
		"msg_id":      trim(anyString(data["msg_id"]), 64),
		"sender_sid":  sid,
		"sender_name": name,
	}
	h.Broadcast(room, "voice_message", payload, sid)
	h.log.Printf("voice name=%s room=%s duration=%.1fs bytes=%d", name, room, duration, len(audio))
}

func (h *Hub) handleVoiceChunk(ctx context.Context, sid string, data map[string]any) {
	room, name := h.roomName(sid)
	if room == "" {
		return
	}
	audio := util.StripDataURLBase64(anyString(data["audio"]))
	if audio == "" || len(audio) > h.cfg.MaxChunkBytes {
		return
	}
	if !h.rate.Check(ctx, sid+":live", h.cfg.MaxChunkRate, h.cfg.MsgRateWindow) {
		return
	}
	mime := anyString(data["mime"])
	if !allowedMIME[mime] {
		mime = "audio/webm"
	}
	h.Broadcast(room, "voice_chunk", map[string]any{
		"audio":       audio,
		"mime":        mime,
		"stream_id":   trim(anyString(data["stream_id"]), 32),
		"seq":         int(anyFloat(data["seq"])),
		"sender_sid":  sid,
		"sender_name": name,
	}, sid)
}

func (h *Hub) handleVoiceStreamEnd(sid string, data map[string]any) {
	room, name := h.roomName(sid)
	if room == "" {
		return
	}
	h.Broadcast(room, "voice_stream_end", map[string]any{
		"stream_id":   trim(anyString(data["stream_id"]), 32),
		"sender_sid":  sid,
		"sender_name": name,
	}, sid)
}

func (h *Hub) handleAIChat(ctx context.Context, sid string, data map[string]any) {
	msgID := trim(anyString(data["msg_id"]), 80)
	if !h.rate.Check(ctx, sid+":ai", h.cfg.MaxAIChatRate, h.cfg.MsgRateWindow) {
		h.Send(sid, "ai_chat_error", map[string]any{"msg_id": msgID, "error": "Slow down — too many AI messages"})
		return
	}
	room, name := h.roomName(sid)
	if room == "" {
		room = util.CleanRoom(anyString(data["room"]), h.cfg.MaxRoomLen)
		if room == "" {
			room = "AI-CHAT"
		}
	}
	if name == "" || name == sidPrefix(sid) {
		name = util.CleanName(anyString(data["username"]), "guest", h.cfg.MaxNameLen)
	}
	data["room"] = room
	data["username"] = name
	h.Send(sid, "ai_chat_typing", map[string]any{"msg_id": msgID, "on": true})
	go func() {
		requestCtx, cancel := context.WithTimeout(context.Background(), h.cfg.AIChatTimeout+5*time.Second)
		defer cancel()
		res := h.ai.BuildChat(requestCtx, data)
		h.Send(sid, "ai_chat_typing", map[string]any{"msg_id": msgID, "on": false})
		if !res.OK {
			h.Send(sid, "ai_chat_error", map[string]any{"msg_id": msgID, "error": res.Error})
			return
		}
		h.Send(sid, "ai_chat_response", map[string]any{"msg_id": msgID, "text": res.Text, "sender_name": "AI Assistant"})
	}()
}

func decodeMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return map[string]any{}
	}
	return data
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), "\"")
}

func anyFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func trim(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > limit {
		return string(r[:limit])
	}
	return string(r)
}
