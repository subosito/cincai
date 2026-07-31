package xai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/adaptersdk/upstreamauth"
	"github.com/subosito/cincai/observability"
)

// TranscriptionHandler maps OpenAI /v1/audio/transcriptions → xAI POST /v1/stt.
//
// Clients keep the Whisper-compatible ingress path and model id (e.g. grok-stt);
// this adapter rebuilds multipart for xAI (file last; optional language/format/keyterm).
type TranscriptionHandler struct{}

func (h *TranscriptionHandler) Protocol() string { return "xai-stt" }

func (h *TranscriptionHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	ct := ""
	if hdr != nil {
		ct = hdr.Get("Content-Type")
	}
	fileName, fileCT, fileData, fields, err := parseOpenAITranscription(ct, raw)
	if err != nil {
		return nil, fmt.Errorf("xai-stt: %w", err)
	}
	if len(fileData) == 0 {
		return nil, fmt.Errorf("xai-stt: file is required")
	}

	var out bytes.Buffer
	w := multipart.NewWriter(&out)
	// xAI expects option fields before file.
	if lang := firstField(fields, "language"); lang != "" {
		_ = w.WriteField("language", lang)
	}
	// Inverse text normalization when language is set (OpenAI has no direct equivalent).
	if lang := firstField(fields, "language"); lang != "" {
		_ = w.WriteField("format", "true")
	}
	// OpenAI prompt → keyterm bias (best-effort; single prompt string).
	if prompt := firstField(fields, "prompt"); prompt != "" {
		_ = w.WriteField("keyterm", prompt)
	}
	if fileCT == "" {
		fileCT = "application/octet-stream"
	}
	if fileName == "" {
		fileName = "audio.bin"
	}
	partHdr := make(textproto.MIMEHeader)
	partHdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(fileName)))
	partHdr.Set("Content-Type", fileCT)
	fw, err := w.CreatePart(partHdr)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(fileData); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	base := strings.TrimRight(t.BaseURL, "/")
	targetURL := base + "/stt"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, &out)
	if err != nil {
		return nil, err
	}
	// Multipart body — do not force JSON content-type.
	if err := upstreamauth.Apply(t, req, hdr, upstreamauth.BearerDefault()); err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Del("Content-Length")
	return observability.HTTPDo(ctx, client, req)
}

func parseOpenAITranscription(contentType string, raw []byte) (fileName, fileCT string, fileData []byte, fields map[string][]string, err error) {
	fields = map[string][]string{}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return "", "", nil, nil, fmt.Errorf("expected multipart body, got %q", contentType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", "", nil, nil, fmt.Errorf("multipart boundary missing")
	}
	r := multipart.NewReader(bytes.NewReader(raw), boundary)
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", nil, nil, err
		}
		name := part.FormName()
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return "", "", nil, nil, err
		}
		if name == "file" || part.FileName() != "" {
			fileName = part.FileName()
			if fileName == "" {
				fileName = "audio.bin"
			}
			fileCT = part.Header.Get("Content-Type")
			fileData = data
			continue
		}
		if name != "" {
			fields[name] = append(fields[name], string(data))
		}
	}
	return fileName, fileCT, fileData, fields, nil
}

func firstField(fields map[string][]string, key string) string {
	vals := fields[key]
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(filepath.Base(s), `"`, ``)
}
