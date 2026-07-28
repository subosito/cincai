package gemini

import (
	"encoding/base64"
	"testing"
)

func TestParseDataURL_ok(t *testing.T) {
	t.Parallel()
	mime, b64, err := ParseDataURL("data:image/png;base64,AAAA")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || b64 != "AAAA" {
		t.Fatalf("%s %q", mime, b64)
	}
}

func TestParseDataURL_tooLarge(t *testing.T) {
	t.Parallel()
	raw := make([]byte, MaxInlineMediaBytes+1)
	b64 := base64.StdEncoding.EncodeToString(raw)
	_, _, err := ParseDataURL("data:image/png;base64," + b64)
	if err == nil || !contains(err.Error(), "too large") {
		t.Fatalf("want too large error, got %v", err)
	}
}

func TestMediaFromAnthropicSource(t *testing.T) {
	t.Parallel()
	med, err := MediaFromAnthropicSource(map[string]any{
		"type":       "base64",
		"media_type": "image/png",
		"data":       "AAAA",
	})
	if err != nil || med == nil || med.Inline == nil || med.Inline.MimeType != "image/png" {
		t.Fatalf("med=%+v err=%v", med, err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
