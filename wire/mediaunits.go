package wire

import (
	"encoding/json"

	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/observability"
)

// requestUnits counts billable media units from the request for wires whose
// response carries no token usage. Read from the request (not the response)
// because it's in the standard wire shape — the same whether the upstream is
// passthrough or a cincai adapter — and it avoids buffering large (e.g. base64)
// media responses just to count them. A cincai adapter that knows a truer count
// can still override rec.Usage during Forward; that write wins.
//
//   - openai-images:  the `n` requested (default 1) → "image"
//   - openai-tts:     the `input` character count   → "character"
//
// Video is async (job + poll) and not counted here.
func requestUnits(wire string, raw []byte) (observability.Usage, bool) {
	if len(raw) == 0 {
		return observability.Usage{}, false
	}
	switch wire {
	case catalog.WireOpenAIImagesGen:
		var req struct {
			N *int `json:"n"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return observability.Usage{}, false
		}
		n := 1
		if req.N != nil && *req.N > 0 {
			n = *req.N
		}
		return observability.Usage{Units: n, Unit: "image"}, true
	case catalog.WireOpenAIAudioSpeech:
		var req struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(raw, &req); err != nil || req.Input == "" {
			return observability.Usage{}, false
		}
		return observability.Usage{Units: len([]rune(req.Input)), Unit: "character"}, true
	default:
		return observability.Usage{}, false
	}
}
