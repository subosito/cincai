package observability

import (
	"context"
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

func TestUsageSink_nilIsNoop(t *testing.T) {
	SetUsageSink(nil)
	// Must not panic with no sink registered.
	RecordIngress(context.Background(), &Recorder{Wire: "w"}, 200, time.Now())
}
