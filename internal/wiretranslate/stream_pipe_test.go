package wiretranslate

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestTranslateAnthropicStreamToOpenAI_incremental(t *testing.T) {
	// Simulate Anthropic SSE with a delay between frames; client should see
	// OpenAI chunks before the upstream stream fully ends.
	anth := "" +
		"event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-test\"}}\n\n" +
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
}
