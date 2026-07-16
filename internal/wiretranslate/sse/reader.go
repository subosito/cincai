// Package sse parses Server-Sent Event frames.
package sse

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// Frame is one SSE event (event name + concatenated data lines).
type Frame struct {
	Event string
	Data  []byte
}

// ReadFrames yields SSE frames from r until EOF.
func ReadFrames(r io.Reader, fn func(Frame) error) error {
	sc := bufio.NewScanner(r)
	const maxLine = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxLine)

	var cur event
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			if cur.hasData() {
				if err := fn(cur.frame()); err != nil {
					return err
				}
			}
			cur = event{}
			continue
		}
		if bytes.HasPrefix(line, []byte(":")) {
			continue
		}
		if after, ok := bytes.CutPrefix(line, []byte("event:")); ok {
			cur.name = strings.TrimSpace(string(after))
			continue
		}
		if after, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			payload := bytes.TrimSpace(after)
			if len(cur.data) > 0 {
				cur.data = append(cur.data, '\n')
			}
			cur.data = append(cur.data, payload...)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if cur.hasData() {
		return fn(cur.frame())
	}
	return nil
}

type event struct {
	name string
	data []byte
}

func (e *event) hasData() bool { return len(e.data) > 0 }

func (e *event) frame() Frame {
	return Frame{Event: e.name, Data: bytes.Clone(e.data)}
}
