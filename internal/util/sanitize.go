package util

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

var (
	roomRe   = regexp.MustCompile(`[^A-Z0-9_\-]`)
	deviceRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)
	colorRe  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	ctrlRe   = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

func CleanName(raw, fallback string, limit int) string {
	text := strings.TrimSpace(strings.ReplaceAll(raw, " ", "_"))
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len([]rune(out)) > limit {
		out = string([]rune(out)[:limit])
	}
	if out == "" {
		f := []rune(fallback)
		if len(f) > limit {
			return string(f[:limit])
		}
		return fallback
	}
	return out
}

func CleanRoom(raw string, limit int) string {
	text := strings.ToUpper(strings.TrimSpace(raw))
	text = roomRe.ReplaceAllString(text, "")
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

func CleanDevice(raw string) string {
	text := deviceRe.ReplaceAllString(strings.TrimSpace(raw), "")
	if len(text) > 128 {
		return text[:128]
	}
	return text
}

func CleanColor(raw string) string {
	if colorRe.MatchString(strings.TrimSpace(raw)) {
		return strings.TrimSpace(raw)
	}
	return "#007aff"
}

func CleanSmallText(raw string, limit int) string {
	text := ctrlRe.ReplaceAllString(raw, "")
	text = strings.TrimSpace(text)
	if len([]rune(text)) > limit {
		return string([]rune(text)[:limit])
	}
	return text
}

func StripDataURLBase64(value string) string {
	text := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(text), "data:") {
		if idx := strings.Index(text, ","); idx >= 0 {
			return strings.TrimSpace(text[idx+1:])
		}
	}
	return text
}

func RandomID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return prefix + "fallback"
	}
	return prefix + hex.EncodeToString(b)
}

func RedactURL(url string) string {
	if url == "" {
		return ""
	}
	re := regexp.MustCompile(`(^[a-zA-Z][a-zA-Z0-9+.-]*://[^:/@]+:)[^@]+(@)`)
	return re.ReplaceAllString(url, `$1***$2`)
}
