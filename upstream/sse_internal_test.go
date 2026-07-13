package upstream

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

// A normal SSE line reads back intact; an upstream that streams past the cap without a
// newline is rejected instead of buffering unboundedly.
func TestReadSSELine(t *testing.T) {
	// Normal line, newline-terminated.
	br := bufio.NewReader(strings.NewReader("data: hello\n"))
	line, err := readSSELine(br)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if line != "data: hello\n" {
		t.Fatalf("line=%q", line)
	}

	// Oversized line with no newline: must error, not OOM.
	huge := io.LimitReader(neverEnding{}, maxSSELineBytes+1024)
	line, err = readSSELine(bufio.NewReader(huge))
	if err == nil {
		t.Fatal("expected error for oversized SSE line")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("want size-cap error, got %v", err)
	}
	if len(line) != 0 {
		t.Fatalf("oversized line should return empty, got %d bytes", len(line))
	}
}

// neverEnding yields 'a' forever (no '\n').
type neverEnding struct{}

func (neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}
