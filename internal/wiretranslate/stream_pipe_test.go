package wiretranslate

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// closeTrackingReader fails Read after Close — used to catch the bug where
// Forward deferred-closed the upstream body when returning an async stream pipe.
type closeTrackingReader struct {
	r      io.Reader
	closed atomic.Bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return c.r.Read(p)
}

func (c *closeTrackingReader) Close() error {
	c.closed.Store(true)
	return nil
}

func TestTranslateAnthropicStreamToOpenAI_incremental(t *testing.T) {
	// Simulate Anthropic SSE with a delay between frames; client should see
	// OpenAI chunks before the upstream stream fully ends.
	anth := "" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-test\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1,\"cache_read_input_tokens\":4}}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	pr, pw := io.Pipe()
	go func() {
		// Write in two halves with a pause so incremental is meaningful.
		half := len(anth) / 2
		_, _ = pw.Write([]byte(anth[:half]))
		time.Sleep(20 * time.Millisecond)
		_, _ = pw.Write([]byte(anth[half:]))
		_ = pw.Close()
	}()

	resp, err := translateAnthropicStreamToOpenAI(pr, "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Read progressively; should not block until full upstream if pipe works.
	var got strings.Builder
	buf := make([]byte, 256)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timeout reading client stream")
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			got.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	out := got.String()
	if !strings.Contains(out, "Hello") || !strings.Contains(out, " world") {
		t.Fatalf("missing text deltas: %q", out)
	}
	if !strings.Contains(out, "chat.completion.chunk") {
		t.Fatalf("not openai chunks: %q", out)
	}
	if !strings.Contains(out, "[DONE]") {
		t.Fatalf("missing DONE: %q", out)
	}
	// Usage must appear so gateway metering can parse OpenAI-shaped stream frames.
	if !strings.Contains(out, `"prompt_tokens":10`) {
		t.Fatalf("missing prompt_tokens in stream: %q", out)
	}
	if !strings.Contains(out, `"completion_tokens":2`) {
		t.Fatalf("missing completion_tokens in stream: %q", out)
	}
	if !strings.Contains(out, `"cached_tokens":4`) {
		t.Fatalf("missing cache read in stream: %q", out)
	}
}

// Returning a stream pipe must keep the upstream reader open until the pipe
// finishes. Closing it on Forward return (old pattern) yields an empty client stream.
func TestTranslateAnthropicStreamToOpenAI_upstreamStaysOpenAfterReturn(t *testing.T) {
	anth := "" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"m\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"alive\"}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	// Slow upstream so the goroutine has not finished when translate returns.
	pr, pw := io.Pipe()
	src := &closeTrackingReader{r: pr}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(pw, anth)
		_ = pw.Close()
	}()

	resp, err := translateAnthropicStreamToOpenAI(src, "m")
	if err != nil {
		t.Fatal(err)
	}
	// Immediately after return the upstream must still be open (pipe owns it).
	if src.closed.Load() {
		t.Fatal("upstream body closed before stream pipe drained")
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "alive") {
		t.Fatalf("empty or incomplete stream: %q", out)
	}
	// After client drain, ownership transfer should close upstream.
	deadline := time.Now().Add(2 * time.Second)
	for !src.closed.Load() {
		if time.Now().After(deadline) {
			t.Fatal("upstream body never closed after pipe finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
