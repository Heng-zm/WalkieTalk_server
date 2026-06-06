package realtime

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"walkietalk-go/internal/ai"
	"walkietalk-go/internal/config"
	"walkietalk-go/internal/store"
	"walkietalk-go/internal/util"
)

type Hub struct {
	cfg  config.Config
	rate *store.RateStore
	ai   *ai.Client
	log  *log.Logger

	register   chan *Client
	unregister chan *Client

	mu      sync.RWMutex
	clients map[string]*Client
	users   map[string]*User
	rooms   map[string]map[string]bool
	screens map[string]*ScreenState
	quality map[string]*QualityState
	closed  bool
}

func NewHub(cfg config.Config, rate *store.RateStore, aiClient *ai.Client, logger *log.Logger) *Hub {
	return &Hub{
		cfg:        cfg,
		rate:       rate,
		ai:         aiClient,
		log:        logger,
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		clients:    make(map[string]*Client),
		users:      make(map[string]*User),
		rooms:      make(map[string]map[string]bool),
		screens:    make(map[string]*ScreenState),
		quality:    make(map[string]*QualityState),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.register:
			h.addClient(c)
		case c := <-h.unregister:
			h.removeClient(c.SID, "disconnect")
		}
	}
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	for _, c := range h.clients {
		close(c.Send)
	}
	h.mu.Unlock()
}

func (h *Hub) addClient(c *Client) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(c.Send)
		return
	}
	h.clients[c.SID] = c
	h.quality[c.SID] = &QualityState{Pending: make(map[string]time.Time)}
	h.mu.Unlock()
	h.Send(c.SID, "connected", map[string]any{"sid": c.SID, "instance": h.cfg.InstanceID})
	go h.qualityLoop(c.SID)
	h.log.Printf("connect sid=%s", c.SID)
}

func (h *Hub) removeClient(sid, reason string) {
	room, name := h.leaveNoBroadcast(sid)
	h.stopScreenShareForSID(sid, reason)
	h.mu.Lock()
	if c, ok := h.clients[sid]; ok {
		delete(h.clients, sid)
		close(c.Send)
	}
	delete(h.quality, sid)
	h.mu.Unlock()
	if room != "" {
		h.Broadcast(room, "peer_left", map[string]any{"sid": sid, "name": name}, sid)
	}
	h.log.Printf("disconnect sid=%s room=%s", sid, room)
}

func (h *Hub) Send(sid, event string, data any) {
	h.mu.RLock()
	c := h.clients[sid]
	h.mu.RUnlock()
	if c == nil {
		return
	}
	select {
	case c.Send <- makeEnvelope(event, data):
	default:
		go h.removeClient(sid, "slow_consumer")
	}
}

func (h *Hub) Broadcast(room, event string, data any, skipSID string) {
	env := makeEnvelope(event, data)
	h.mu.RLock()
	sids := make([]string, 0, len(h.rooms[room]))
	for sid := range h.rooms[room] {
		if sid != skipSID {
			sids = append(sids, sid)
		}
	}
	clients := make([]*Client, 0, len(sids))
	for _, sid := range sids {
		if c := h.clients[sid]; c != nil {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.Send <- env:
		default:
			go h.removeClient(c.SID, "slow_consumer")
		}
	}
}

func (h *Hub) BroadcastAll(event string, data any) {
	env := makeEnvelope(event, data)
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.Send <- env:
		default:
			go h.removeClient(c.SID, "slow_consumer")
		}
	}
}

func (h *Hub) Stats() map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rooms := make(map[string]int, len(h.rooms))
	for room, sids := range h.rooms {
		rooms[room] = len(sids)
	}
	screens := make(map[string]*ScreenState, len(h.screens))
	for room, state := range h.screens {
		screens[room] = state
	}
	return map[string]any{
		"instance":      h.cfg.InstanceID,
		"local_users":   len(h.users),
		"local_rooms":   rooms,
		"screen_shares": screens,
		"quality_tasks": len(h.quality),
	}
}

func (h *Hub) RoomsSnapshot() map[string]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rooms := make(map[string]int, len(h.rooms))
	for room, sids := range h.rooms {
		rooms[room] = len(sids)
	}
	return rooms
}

func (h *Hub) ScreensSnapshot() map[string]*ScreenState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]*ScreenState, len(h.screens))
	for k, v := range h.screens {
		out[k] = v
	}
	return out
}

func (h *Hub) Members(room string) []RoomMember {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.membersLocked(room)
}

func (h *Hub) membersLocked(room string) []RoomMember {
	sids := make([]string, 0, len(h.rooms[room]))
	for sid := range h.rooms[room] {
		sids = append(sids, sid)
	}
	sort.Strings(sids)
	out := make([]RoomMember, 0, len(sids))
	for _, sid := range sids {
		if u := h.users[sid]; u != nil {
			out = append(out, RoomMember{SID: sid, Name: u.Name})
		}
	}
	return out
}

func (h *Hub) roomName(sid string) (string, string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if u := h.users[sid]; u != nil {
		return u.Room, u.Name
	}
	return "", sidPrefix(sid)
}

func (h *Hub) leaveNoBroadcast(sid string) (string, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	u := h.users[sid]
	if u == nil {
		return "", sidPrefix(sid)
	}
	room, name := u.Room, u.Name
	if h.rooms[room] != nil {
		delete(h.rooms[room], sid)
		if len(h.rooms[room]) == 0 {
			delete(h.rooms, room)
		}
	}
	delete(h.users, sid)
	return room, name
}

func makeEnvelope(event string, data any) Envelope {
	if data == nil {
		return Envelope{Event: event}
	}
	b, err := json.Marshal(data)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"error": "marshal failed"})
	}
	return Envelope{Event: event, Data: b}
}

func sidPrefix(sid string) string {
	if len(sid) <= 6 {
		return sid
	}
	return sid[:6]
}

func (h *Hub) qualityLoop(sid string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		nonce := util.RandomID("q_")
		h.mu.Lock()
		q := h.quality[sid]
		if q == nil {
			h.mu.Unlock()
			return
		}
		q.Pending[nonce] = time.Now()
		h.mu.Unlock()
		h.Send(sid, "quality_ping", map[string]any{"nonce": nonce})
		time.AfterFunc(5*time.Second, func() {
			h.mu.Lock()
			q := h.quality[sid]
			if q == nil {
				h.mu.Unlock()
				return
			}
			if _, ok := q.Pending[nonce]; ok {
				delete(q.Pending, nonce)
				q.Cycles = appendBoundedBool(q.Cycles, false, 5)
			}
			score, rtt, drop, jitter := qualityScore(q.RTTs, q.Cycles)
			h.mu.Unlock()
			h.Send(sid, "quality_update", map[string]any{"score": score, "latency_ms": rtt, "drop_pct": drop, "jitter_ms": jitter})
		})
	}
}

func (h *Hub) handleQualityPong(sid string, data map[string]any) {
	nonce, _ := data["nonce"].(string)
	if nonce == "" {
		return
	}
	h.mu.Lock()
	q := h.quality[sid]
	if q == nil {
		h.mu.Unlock()
		return
	}
	sent, ok := q.Pending[nonce]
	if ok {
		delete(q.Pending, nonce)
		q.RTTs = appendBoundedFloat(q.RTTs, float64(time.Since(sent).Milliseconds()), 5)
		q.Cycles = appendBoundedBool(q.Cycles, true, 5)
	}
	score, rtt, drop, jitter := qualityScore(q.RTTs, q.Cycles)
	h.mu.Unlock()
	h.Send(sid, "quality_update", map[string]any{"score": score, "latency_ms": rtt, "drop_pct": drop, "jitter_ms": jitter})
}

func qualityScore(rtts []float64, cycles []bool) (int, float64, float64, float64) {
	if len(rtts) == 0 {
		return 100, 0, 0, 0
	}
	s := append([]float64(nil), rtts...)
	sort.Float64s(s)
	n := len(s)
	median := s[n/2]
	if n%2 == 0 {
		median = (s[n/2-1] + s[n/2]) / 2
	}
	mean := 0.0
	for _, v := range s {
		mean += v
	}
	mean /= float64(n)
	jitter := 0.0
	if n >= 2 {
		for _, v := range s {
			jitter += math.Pow(v-mean, 2)
		}
		jitter = math.Sqrt(jitter / float64(n-1))
	}
	drops := 0
	for _, ok := range cycles {
		if !ok {
			drops++
		}
	}
	dropPct := 0.0
	if len(cycles) > 0 {
		dropPct = float64(drops) / float64(len(cycles)) * 100
	}
	latScore := 50.0
	if median > 100 && median <= 400 {
		latScore = 50.0 - (median-100)/300*25
	} else if median > 400 {
		latScore = math.Max(0, 25-(median-400)/200*25)
	}
	dropScore := math.Max(0, 30-dropPct/50*30)
	jitScore := 20.0
	if jitter > 20 && jitter <= 150 {
		jitScore = 20 - (jitter-20)/130*10
	} else if jitter > 150 {
		jitScore = math.Max(0, 10-(jitter-150)/100*10)
	}
	score := int(math.Round(latScore + dropScore + jitScore))
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, round1(median), round1(dropPct), round1(jitter)
}

func appendBoundedFloat(items []float64, value float64, max int) []float64 {
	items = append(items, value)
	if len(items) > max {
		return items[len(items)-max:]
	}
	return items
}

func appendBoundedBool(items []bool, value bool, max int) []bool {
	items = append(items, value)
	if len(items) > max {
		return items[len(items)-max:]
	}
	return items
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
