package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"walkietalk-go/internal/config"
	"walkietalk-go/internal/util"
)

type Client struct {
	cfg  config.Config
	log  *log.Logger
	http *http.Client
}

type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type ChatRequest struct {
	Text     string        `json:"text"`
	Message  string        `json:"message"`
	Prompt   string        `json:"prompt"`
	Username string        `json:"username"`
	Room     string        `json:"room"`
	History  []ChatMessage `json:"history"`
	Source   string        `json:"source"`
}

type ChatResponse struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

func NewClient(cfg config.Config, logger *log.Logger) *Client {
	return &Client{cfg: cfg, log: logger, http: &http.Client{Timeout: cfg.AITimeout}}
}

func (c *Client) BuildChat(ctx context.Context, raw map[string]any) ChatResponse {
	text := cleanAIText(anyString(raw["text"]), c.cfg.MaxAITextLen)
	if text == "" {
		text = cleanAIText(anyString(raw["message"]), c.cfg.MaxAITextLen)
	}
	if text == "" {
		text = cleanAIText(anyString(raw["prompt"]), c.cfg.MaxAITextLen)
	}
	if text == "" {
		return ChatResponse{OK: false, Error: "Message is empty"}
	}
	username := util.CleanName(anyString(raw["username"]), "guest", c.cfg.MaxNameLen)
	room := util.CleanRoom(anyString(raw["room"]), c.cfg.MaxRoomLen)
	if room == "" {
		room = "AI-CHAT"
	}
	history := cleanHistory(raw["history"], c.cfg.MaxAIHistory)

	reply, err := c.CallChat(ctx, text, username, room, history)
	if err != nil {
		return ChatResponse{OK: false, Error: err.Error()}
	}
	return ChatResponse{OK: true, Text: reply}
}

func (c *Client) CallChat(ctx context.Context, text, username, room string, history []ChatMessage) (string, error) {
	urls := make([]string, 0, 1)
	if c.cfg.AIChatURL != "" {
		urls = append(urls, c.cfg.AIChatURL)
	} else if c.cfg.AIAssistantURL != "" {
		urls = append(urls, c.cfg.AIAssistantURL)
	}
	if len(urls) == 0 {
		return "AI chat backend is not configured yet. Set AI_CHAT_URL or AI_ASSISTANT_URL.", nil
	}
	if c.cfg.AIAssistantAPIKey == "" && c.cfg.AIChatURL == "" && c.cfg.AIAssistantURL != "" {
		return "AI assistant API key is missing. Set AI_ASSISTANT_API_KEY to the same value as AI_API_KEY on the bot-voice server, or set AI_CHAT_URL to a text endpoint that does not require that key.", nil
	}

	payload := ChatRequest{
		Text: text, Message: text, Prompt: text,
		Username: username, Room: room, History: history, Source: "walkietalk_go_ai_chat",
	}
	var lastErr string
	for _, url := range urls {
		reply, err := c.postJSONWithRetry(ctx, url, payload, c.cfg.AIChatTimeout)
		if err == nil && strings.TrimSpace(reply) != "" {
			return reply, nil
		}
		if err != nil {
			lastErr = err.Error()
		} else {
			lastErr = "AI backend returned no text"
		}
	}
	if c.cfg.AIChatURL != "" {
		return "", errors.New(lastErr)
	}
	setup := "AI assistant endpoint did not return a text chat reply. Set AI_ASSISTANT_API_KEY to the same value as AI_API_KEY on the bot-voice server, or set AI_CHAT_URL to a dedicated text chat endpoint."
	if lastErr != "" {
		return setup + " Last error: " + trim(lastErr, 180), nil
	}
	return setup, nil
}

func (c *Client) postJSONWithRetry(ctx context.Context, url string, payload any, timeout time.Duration) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 1; attempt <= c.cfg.AIRetryAttempts; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			cancel()
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		if c.cfg.AIAssistantAPIKey != "" {
			req.Header.Set("X-Api-Key", c.cfg.AIAssistantAPIKey)
			req.Header.Set("Authorization", "Bearer "+c.cfg.AIAssistantAPIKey)
		}
		resp, err := c.http.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			if attempt < c.cfg.AIRetryAttempts {
				time.Sleep(backoff(attempt, c.cfg.AIRetryBaseDelay))
				continue
			}
			return "", err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("AI backend HTTP %d: %s", resp.StatusCode, trim(string(data), 180))
			if retryableStatus(resp.StatusCode) && attempt < c.cfg.AIRetryAttempts {
				time.Sleep(backoff(attempt, c.cfg.AIRetryBaseDelay))
				continue
			}
			return "", lastErr
		}
		if reply := extractReply(data); reply != "" {
			return reply, nil
		}
		lastErr = errors.New("AI backend returned JSON without text/reply/response")
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("AI backend request failed")
}

func extractReply(data []byte) string {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return cleanAIText(string(data), 6000)
	}
	for _, k := range []string{"text", "reply", "response", "answer", "message", "content"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return cleanAIText(v, 6000)
		}
	}
	if choices, ok := m["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if msg, ok := first["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok {
					return cleanAIText(content, 6000)
				}
			}
			if text, ok := first["text"].(string); ok {
				return cleanAIText(text, 6000)
			}
		}
	}
	return ""
}

func cleanAIText(text string, limit int) string {
	text = util.CleanSmallText(text, limit)
	if len([]rune(text)) > limit {
		return string([]rune(text)[:limit])
	}
	return text
}

func cleanHistory(value any, max int) []ChatMessage {
	if max <= 0 {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	start := 0
	if len(items) > max {
		start = len(items) - max
	}
	out := make([]ChatMessage, 0, len(items)-start)
	for _, item := range items[start:] {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(anyString(m["role"]))
		if role != "user" && role != "assistant" {
			continue
		}
		text := cleanAIText(firstNonEmpty(anyString(m["text"]), anyString(m["content"])), 800)
		if text != "" {
			out = append(out, ChatMessage{Role: role, Text: text})
		}
	}
	return out
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func trim(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

func retryableStatus(code int) bool {
	switch code {
	case 408, 409, 425, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func backoff(attempt int, base time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base * time.Duration(1<<(attempt-1))
	if d > 4*time.Second {
		d = 4 * time.Second
	}
	return d
}
