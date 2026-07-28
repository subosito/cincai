package gemini

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// MaxInlineMediaBytes is the max decoded size for a single data-URL media part.
// Larger payloads get a clear error instead of opaque upstream failures.
const MaxInlineMediaBytes = 20 << 20 // 20 MiB

// MaxUpstreamBodyBytes is the max response body we read from Vertex / generateContent-style APIs.
const MaxUpstreamBodyBytes = 32 << 20 // 32 MiB

// MediaResult is a single Gemini-style media part (inline base64 or remote URI).
type MediaResult struct {
	Inline *InlineData
	File   *FileData
}

// ParseDataURL parses a data:[mime];base64,<payload> URL.
func ParseDataURL(dataURL string) (mime, b64 string, err error) {
	rest := strings.TrimPrefix(dataURL, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", "", fmt.Errorf("invalid data URL (no comma)")
	}
	meta, payload := rest[:comma], rest[comma+1:]
	if !strings.Contains(meta, ";base64") {
		return "", "", fmt.Errorf("data URL must be base64-encoded")
	}
	mime = strings.TrimSpace(strings.Split(meta, ";")[0])
	if mime == "" {
		mime = "application/octet-stream"
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", "", fmt.Errorf("empty data URL payload")
	}
	// Validate size without holding decoded bytes longer than needed.
	decoded, decErr := base64.StdEncoding.DecodeString(payload)
	if decErr != nil {
		// URL-safe / raw variants sometimes appear; try RawStdEncoding.
		decoded, decErr = base64.RawStdEncoding.DecodeString(payload)
		if decErr != nil {
			return "", "", fmt.Errorf("invalid base64 data URL payload: %w", decErr)
		}
	}
	if len(decoded) > MaxInlineMediaBytes {
		return "", "", fmt.Errorf("media part too large: %d bytes (max %d); use a smaller file or File API URI",
			len(decoded), MaxInlineMediaBytes)
	}
	return mime, payload, nil
}

// ExtractMediaURL pulls a media URL from an OpenAI (or Anthropic-shaped) content part map.
func ExtractMediaURL(p map[string]any, urlKey string) string {
	for _, key := range []string{urlKey, "image_url", "video_url", "audio_url", "url", "source"} {
		v, ok := p[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if s := strings.TrimSpace(x); s != "" {
				return s
			}
		case map[string]any:
			if u, ok := x["url"].(string); ok && strings.TrimSpace(u) != "" {
				return strings.TrimSpace(u)
			}
			if u, ok := x["data"].(string); ok && strings.TrimSpace(u) != "" {
				media, _ := x["media_type"].(string)
				if media == "" {
					media, _ = x["mime_type"].(string)
				}
				if media == "" {
					media = "application/octet-stream"
				}
				return "data:" + media + ";base64," + strings.TrimSpace(u)
			}
		}
	}
	return ""
}

// MIMEFromPath guesses a MIME type from a URL path extension.
func MIMEFromPath(rawURL, defaultMIME string) string {
	mime := defaultMIME
	u, err := url.Parse(rawURL)
	if err != nil {
		return mime
	}
	path := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(path, ".webm"):
		return "video/webm"
	case strings.HasSuffix(path, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(path, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(path, ".ogg"), strings.HasSuffix(path, ".opus"):
		return "audio/ogg"
	default:
		return mime
	}
}

// MediaFromOpenAIPart maps an OpenAI multimodal content part to inlineData or fileData.
// urlKey is the preferred nested key ("image_url", "video_url", "audio_url").
func MediaFromOpenAIPart(p map[string]any, urlKey, defaultMIME string) (*MediaResult, error) {
	rawURL := ExtractMediaURL(p, urlKey)
	if rawURL == "" {
		return nil, nil
	}
	if strings.HasPrefix(rawURL, "data:") {
		mime, b64, err := ParseDataURL(rawURL)
		if err != nil {
			return nil, err
		}
		if mime == "" {
			mime = defaultMIME
		}
		return &MediaResult{Inline: &InlineData{MimeType: mime, Data: b64}}, nil
	}
	return &MediaResult{
		File: &FileData{
			MimeType: MIMEFromPath(rawURL, defaultMIME),
			FileURI:  rawURL,
		},
	}, nil
}

// MediaFromAnthropicSource maps {"type":"image","source":{...}} style blocks.
func MediaFromAnthropicSource(source map[string]any) (*MediaResult, error) {
	if source == nil {
		return nil, nil
	}
	typ, _ := source["type"].(string)
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "base64", "":
		media, _ := source["media_type"].(string)
		if media == "" {
			media, _ = source["mime_type"].(string)
		}
		if media == "" {
			media = "image/png"
		}
		data, _ := source["data"].(string)
		data = strings.TrimSpace(data)
		if data == "" {
			return nil, nil
		}
		// Reuse size check via ParseDataURL shape.
		mime, b64, err := ParseDataURL("data:" + media + ";base64," + data)
		if err != nil {
			return nil, err
		}
		return &MediaResult{Inline: &InlineData{MimeType: mime, Data: b64}}, nil
	case "url":
		u, _ := source["url"].(string)
		u = strings.TrimSpace(u)
		if u == "" {
			return nil, nil
		}
		if strings.HasPrefix(u, "data:") {
			mime, b64, err := ParseDataURL(u)
			if err != nil {
				return nil, err
			}
			return &MediaResult{Inline: &InlineData{MimeType: mime, Data: b64}}, nil
		}
		return &MediaResult{
			File: &FileData{MimeType: MIMEFromPath(u, "image/png"), FileURI: u},
		}, nil
	default:
		return nil, nil
	}
}

// ContentPartFromMedia builds a ContentPart from MediaResult.
func ContentPartFromMedia(m *MediaResult) ContentPart {
	if m == nil {
		return ContentPart{}
	}
	return ContentPart{InlineData: m.Inline, FileData: m.File}
}
