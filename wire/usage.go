package wire

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"

	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/observability"
)

// maxUsageBytes caps the buffered body. Usage is small; this bounds memory on
// large or hostile responses.
const maxUsageBytes = 1 << 20

// usageMeter tees response bytes (via upstream.UsageObserver) and extracts token
// usage on completion. It parses by response shape (OpenAI chat + responses,
// Anthropic, Gemini) and inflates gzip — the same coverage across every token
// wire, no per-vendor branching.
type usageMeter struct {
	buf      []byte
	encoding string // response Content-Encoding, set before streaming begins
}

// usageMeterFor returns a meter for token-bearing wires, or nil (media/other
// wires carry no token usage; unit counts are set at route time instead).
func usageMeterFor(wire string) *usageMeter {
	switch wire {
	case catalog.WireOpenAIChat, catalog.WireOpenAIResponses, catalog.WireAnthropicMsg:
		return &usageMeter{}
	default:
		return nil
	}
}

func (m *usageMeter) Observe(p []byte) {
	m.buf = append(m.buf, p...)
	if len(m.buf) <= maxUsageBytes {
		return
	}
	if strings.Contains(strings.ToLower(m.encoding), "gzip") {
		m.buf = m.buf[:maxUsageBytes] // gzip needs the header at the front
	} else {
		m.buf = m.buf[len(m.buf)-maxUsageBytes:] // plain: usage is at the tail
	}
}

// Result parses the buffered body. Zero Usage means the upstream reported none
// (e.g. a stream without stream_options.include_usage).
func (m *usageMeter) Result() observability.Usage {
	u := parseResponseUsage(decodeContentEncoding(m.encoding, m.buf))
	return observability.Usage{
		InputTokens:      int(u.input),
		OutputTokens:     int(u.output),
		CacheReadTokens:  int(u.cacheRead),
		CacheWriteTokens: int(u.cacheWrite),
	}
}

// respUsage is the intermediate token tally from the shared proxy usage parser.
type respUsage struct{ input, output, cacheRead, cacheWrite int64 }

func (u respUsage) hasTokens() bool {
	return u.input > 0 || u.output > 0 || u.cacheRead > 0 || u.cacheWrite > 0
}

func parseResponseUsage(body []byte) respUsage {
	if len(body) == 0 {
		return respUsage{}
	}
	if isEventStream(body) {
		return parseSSEUsage(body)
	}
	return parseJSONUsage(body)
}

func decodeContentEncoding(contentEncoding string, body []byte) []byte {
	if !strings.Contains(strings.ToLower(contentEncoding), "gzip") {
		return body
	}
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body // not actually gzip-framed
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return body
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, 64<<20))
	if len(out) == 0 && err != nil {
		return body
	}
	return out // partial inflate still yields the leading message_start usage
}

func isEventStream(body []byte) bool {
	trim := bytes.TrimSpace(body)
	return bytes.HasPrefix(trim, []byte("event:")) || bytes.Contains(trim, []byte("\nevent:")) ||
		bytes.HasPrefix(trim, []byte("data:"))
}

func parseSSEUsage(body []byte) respUsage {
	var best respUsage
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if u := parseJSONUsage([]byte(data)); u.hasTokens() {
			best = mergeUsage(best, u)
		}
	}
	return best
}

// mergeUsage combines token fields across SSE events — later events win for the
// fields they carry (Anthropic sends input on message_start, output on
// message_delta) so partial deltas do not zero earlier values.
func mergeUsage(acc, next respUsage) respUsage {
	if next.input > 0 {
		acc.input = next.input
	}
	if next.output > 0 {
		acc.output = next.output
	}
	if next.cacheRead > 0 {
		acc.cacheRead = next.cacheRead
	}
	if next.cacheWrite > 0 {
		acc.cacheWrite = next.cacheWrite
	}
	return acc
}

func parseJSONUsage(body []byte) respUsage {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return respUsage{}
	}
	if u, ok := raw["usage"]; ok {
		if parsed := usageFromJSON(u); parsed.hasTokens() {
			return parsed
		}
	}
	// Anthropic stream: message_start nests usage under "message".
	if msg, ok := raw["message"]; ok {
		var env struct {
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(msg, &env) == nil && len(env.Usage) > 0 {
			if parsed := usageFromJSON(env.Usage); parsed.hasTokens() {
				return parsed
			}
		}
	}
	// Gemini.
	if meta, ok := raw["usageMetadata"]; ok {
		if parsed := usageFromJSON(meta); parsed.hasTokens() {
			return parsed
		}
	}
	// OpenAI responses stream: usage nested under "response".
	if resp, ok := raw["response"]; ok {
		var env struct {
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(resp, &env) == nil && len(env.Usage) > 0 {
			if parsed := usageFromJSON(env.Usage); parsed.hasTokens() {
				return parsed
			}
		}
	}
	return respUsage{}
}

func usageFromJSON(raw json.RawMessage) respUsage {
	var u struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		PromptTokens             int64 `json:"prompt_tokens"`
		CompletionTokens         int64 `json:"completion_tokens"`
		TotalTokens              int64 `json:"total_tokens"`
		TotalTokenCount          int64 `json:"totalTokenCount"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		// OpenAI chat-completions style.
		PromptTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		// OpenAI/xAI Responses API style (grok-*, etc.).
		InputTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return respUsage{}
	}
	in := u.InputTokens
	if in == 0 {
		in = u.PromptTokens
	}
	if in == 0 {
		in = u.TotalTokens
	}
	out := u.OutputTokens
	if out == 0 {
		out = u.CompletionTokens
	}
	cacheRead := u.CacheReadInputTokens
	if cacheRead == 0 && u.PromptTokensDetails != nil {
		cacheRead = u.PromptTokensDetails.CachedTokens
	}
	if cacheRead == 0 && u.InputTokensDetails != nil {
		cacheRead = u.InputTokensDetails.CachedTokens
	}
	if cacheRead == 0 {
		cacheRead = u.CachedContentTokenCount
	}
	return respUsage{input: in, output: out, cacheRead: cacheRead, cacheWrite: u.CacheCreationInputTokens}
}
