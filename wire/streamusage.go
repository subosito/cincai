package wire

import (
	"bytes"
	"encoding/json"

	"github.com/subosito/cincai/catalog"
)

// injectIncludeUsage adds stream_options.include_usage=true to a streaming
// OpenAI chat request that lacks it, so the upstream reports token usage in the
// final SSE frame. Returns the (possibly rewritten) body and whether it injected
// — when true the caller drops that extra usage frame from the client stream so
// the client sees exactly what it would without the injection.
func injectIncludeUsage(wire string, raw []byte) ([]byte, bool) {
	if wire != catalog.WireOpenAIChat || len(raw) == 0 {
		return raw, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, false
	}
	var stream bool
	if err := json.Unmarshal(m["stream"], &stream); err != nil || !stream {
		return raw, false
	}
	opts := map[string]json.RawMessage{}
	if so, ok := m["stream_options"]; ok {
		if err := json.Unmarshal(so, &opts); err != nil {
			return raw, false
		}
		if _, has := opts["include_usage"]; has {
			return raw, false // client set it — respect it, and don't strip
		}
	}
	opts["include_usage"] = json.RawMessage("true")
	so, err := json.Marshal(opts)
	if err != nil {
		return raw, false
	}
	m["stream_options"] = so
	out, err := json.Marshal(m)
	if err != nil {
		return raw, false
	}
	return out, true
}

// isUsageOnlyDataLine reports whether an SSE line is the terminal usage frame
// (empty choices + a usage object) that include_usage produces. Used to strip
// the injected frame so a client that never asked for usage isn't handed a chunk
// with an empty choices array — which naive SDKs index into and crash on.
func isUsageOnlyDataLine(line []byte) bool {
	t := bytes.TrimSpace(line)
	if !bytes.HasPrefix(t, []byte("data:")) {
		return false
	}
	payload := bytes.TrimSpace(t[len("data:"):])
	if len(payload) == 0 || payload[0] != '{' {
		return false
	}
	var chunk struct {
		Choices []json.RawMessage `json:"choices"`
		Usage   json.RawMessage   `json:"usage"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return false
	}
	return len(chunk.Usage) > 0 && len(chunk.Choices) == 0
}
