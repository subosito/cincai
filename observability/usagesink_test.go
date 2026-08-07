package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type captureSink struct{ ev *UsageEvent }

func (c *captureSink) RecordUsage(_ context.Context, ev UsageEvent) { c.ev = &ev }

func TestUsageSink_receivesFinalizedEvent(t *testing.T) {
	c := &captureSink{}
	SetUsageSink(c)
	t.Cleanup(func() { SetUsageSink(nil) })

	rec := &Recorder{
		Wire: "openai-chat-completions", Model: "m", ProviderRef: "p", PrincipalID: "u",
		Usage: Usage{InputTokens: 11, OutputTokens: 22},
	}
	RecordIngress(context.Background(), rec, 200, time.Now())

	if c.ev == nil {
		t.Fatal("usage sink received no event")
	}
	if c.ev.Model != "m" || c.ev.ProviderRef != "p" || c.ev.Status != 200 {
		t.Fatalf("event fields = %+v", *c.ev)
	}
	if c.ev.Usage.InputTokens != 11 || c.ev.Usage.OutputTokens != 22 {
		t.Fatalf("usage = %+v, want in=11 out=22", c.ev.Usage)
	}
}

func TestRecordIngress_includesHostAttribution(t *testing.T) {
	var buf bytes.Buffer
	SetTestLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetTestLogger(slog.Default()) })

	c := &captureSink{}
	SetUsageSink(c)
	t.Cleanup(func() { SetUsageSink(nil) })

	// Stash host labels the same way StashHostAttribution does.
	ctx := context.WithValue(context.Background(), hostAttrKey{}, HostAttribution{
		Actor: "dudu-tim-auk", Session: "sess-1", Component: "turn.chat", CorrelationID: "corr-1",
	})
	RecordIngress(ctx, &Recorder{
		Wire: "anthropic-messages", Model: "claude-sonnet-5", ProviderRef: "anthropic",
		PrincipalID: "host-app", Usage: Usage{InputTokens: 2, OutputTokens: 153},
	}, 200, time.Now())

	out := buf.String()
	for _, want := range []string{
		`"msg":"ingress"`,
		`"actor":"dudu-tim-auk"`,
		`"session":"sess-1"`,
		`"component":"turn.chat"`,
		`"correlation_id":"corr-1"`,
		`"principal_id":"host-app"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in ingress log: %s", want, out)
		}
	}
	if c.ev == nil {
		t.Fatal("usage sink received no event")
	}
	if c.ev.Actor != "dudu-tim-auk" || c.ev.Session != "sess-1" || c.ev.Component != "turn.chat" {
		t.Fatalf("sink event missing host labels: %+v", *c.ev)
	}
}

func TestUsageSink_nilIsNoop(t *testing.T) {
	SetUsageSink(nil)
	// Must not panic with no sink registered.
	RecordIngress(context.Background(), &Recorder{Wire: "w"}, 200, time.Now())
}
