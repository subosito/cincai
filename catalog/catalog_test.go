package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/internal/testfixture"
)

func TestResolveFailover(t *testing.T) {
	path := testfixture.ProvidersYAML()
	cat, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("mock-chat", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != catalog.StrategyFailover || len(plan.Targets) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	target := plan.Targets[0]
	if target.ProviderRef != "mock" || target.UpstreamModel != "echo-model" {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestFailoverPoolOrder(t *testing.T) {
	raw := `
providers:
  primary:
    credential_profile: primary
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://primary
  backup:
    credential_profile: backup
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://backup
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: failover
        providers:
          - provider_ref: primary
            model: mp
          - provider_ref: backup
            model: mb
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("m", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 2 {
		t.Fatalf("targets=%d", len(plan.Targets))
	}
	if plan.Targets[0].ProviderRef != "primary" || plan.Targets[1].ProviderRef != "backup" {
		t.Fatalf("order=%s %s", plan.Targets[0].ProviderRef, plan.Targets[1].ProviderRef)
	}
}

func TestStickyRejected(t *testing.T) {
	raw := `
providers:
  a:
    credential_profile: a
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://a
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: sticky
        providers:
          - provider_ref: a
            model: ma
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("m", catalog.WireOpenAIChat)
	if err == nil {
		t.Fatal("expected sticky strategy error")
	}
}

func TestSurfacePickDeterministic(t *testing.T) {
	raw := `
providers:
  p:
    credential_profile: p
    surfaces:
      z-chat:
        protocol: openai-chat-completions
        base_url: http://z
      a-chat:
        protocol: openai-chat-completions
        base_url: http://a
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: p
            model: x
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := cat.Resolve("m", catalog.WireOpenAIChat)
	second, _ := cat.Resolve("m", catalog.WireOpenAIChat)
	if first.Targets[0].BaseURL != second.Targets[0].BaseURL {
		t.Fatalf("surface pick not deterministic: %s vs %s", first.Targets[0].BaseURL, second.Targets[0].BaseURL)
	}
	if first.Targets[0].BaseURL != "http://a" {
		t.Fatalf("expected sorted surface a-chat, got %s", first.Targets[0].BaseURL)
	}
}

func TestListModels(t *testing.T) {
	path := testfixture.ProvidersYAML()
	cat, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	list := cat.ListModels()
	if list.Object != "list" {
		t.Fatalf("object=%q", list.Object)
	}
	if len(list.Data) != 1 || list.Data[0].ID != "mock-chat" {
		t.Fatalf("data=%+v", list.Data)
	}
	if list.Data[0].Object != "model" || list.Data[0].OwnedBy != "cincai" {
		t.Fatalf("item=%+v", list.Data[0])
	}
	// created is required by the OpenAI model schema; it used to always marshal 0,
	// telling clients every model was published on 1970-01-01.
	if list.Data[0].Created <= 0 {
		t.Fatalf("created=%d, want a real unix timestamp", list.Data[0].Created)
	}
	// Wire metadata for clients (mow) — preferred chat wire + all wires.
	if list.Data[0].Wire == "" {
		t.Fatalf("expected preferred wire, item=%+v", list.Data[0])
	}
	if len(list.Data[0].Wires) == 0 {
		t.Fatalf("expected wires[], item=%+v", list.Data[0])
	}
}

func TestPreferredChatWire(t *testing.T) {
	if got := catalog.PreferredChatWire([]string{catalog.WireAnthropicMsg, catalog.WireOpenAIChat}); got != catalog.WireOpenAIChat {
		t.Fatalf("prefer chat-completions, got %q", got)
	}
	if got := catalog.PreferredChatWire([]string{catalog.WireAnthropicMsg}); got != catalog.WireAnthropicMsg {
		t.Fatalf("got %q", got)
	}
	if got := catalog.PreferredChatWire(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestWireForPathResponses(t *testing.T) {
	wire, ok := catalog.WireForPath("/v1/responses")
	if !ok || wire != catalog.WireOpenAIResponses {
		t.Fatalf("wire=%q ok=%v", wire, ok)
	}
}

func TestWireForPathAnthropicCountTokens(t *testing.T) {
	wire, ok := catalog.WireForPath("/v1/messages/count_tokens")
	if !ok || wire != catalog.WireAnthropicMsg {
		t.Fatalf("wire=%q ok=%v", wire, ok)
	}
}

func TestWireForPathMedia(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/v1/images/generations", catalog.WireOpenAIImagesGen},
		{"/v1/images/edits", catalog.WireOpenAIImagesGen},
		{"/v1/audio/speech", catalog.WireOpenAIAudioSpeech},
		{"/v1/audio/transcriptions", catalog.WireOpenAIAudioTranscriptions},
		{"/v1/videos/generations", catalog.WireOpenAIVideos},
		{"/v1/videos/req_abc", catalog.WireOpenAIVideos},
	}
	for _, tc := range cases {
		got, ok := catalog.WireForPath(tc.path)
		if !ok || got != tc.want {
			t.Fatalf("path=%s got=%q want=%q ok=%v", tc.path, got, tc.want, ok)
		}
	}
}

func TestResolveResponsesWireAlias(t *testing.T) {
	raw := `
providers:
  openai:
    credential_profile: openai
    surfaces:
      responses:
        protocol: openai-responses
        base_url: http://openai
models:
  gpt-responses:
    modalities:
      chat:
        wire: openai-responses
        providers:
          - provider_ref: openai
            surface: responses
            model: gpt-5.5
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("gpt-responses", catalog.WireOpenAIResponses)
	if err != nil {
		t.Fatal(err)
	}
	target := plan.Targets[0]
	if target.Protocol != "openai-responses" || target.UpstreamModel != "gpt-5.5" {
		t.Fatalf("target=%+v", target)
	}
}

func TestTranslateAdapterRequiresExplicitSurface(t *testing.T) {
	raw := `
providers:
  acme:
    credential_profile: acme
    surfaces:
      image:
        adapter: myvendor
        base_url: http://acme
models:
  acme-image:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: acme
            model: acme-image-v1
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cat.Resolve("acme-image", catalog.WireOpenAIImagesGen)
	if err == nil {
		t.Fatal("expected error without explicit surface for translate adapter")
	}

	raw2 := `
providers:
  acme:
    credential_profile: acme
    surfaces:
      image:
        adapter: myvendor
        base_url: http://acme
models:
  acme-image:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: acme
            surface: image
            model: acme-image-v1
`
	tmp2 := filepath.Join(t.TempDir(), "p2.yaml")
	if err := os.WriteFile(tmp2, []byte(raw2), 0o600); err != nil {
		t.Fatal(err)
	}
	cat2, err := catalog.Load(tmp2)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat2.Resolve("acme-image", catalog.WireOpenAIImagesGen)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].Adapter != "myvendor" {
		t.Fatalf("adapter=%q", plan.Targets[0].Adapter)
	}
}

func TestDuplicateWireExpandedToFacets(t *testing.T) {
	// Same wire under one authoring id expands: bare m keeps chat; alt_chat → m:alt_chat.
	raw := `
providers:
  p:
    credential_profile: p
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://p
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: p
            model: x
      alt_chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: p
            model: y
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("m", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatalf("bare after expand: %v", err)
	}
	if plan.Targets[0].UpstreamModel != "x" {
		t.Fatalf("bare model=%q want x", plan.Targets[0].UpstreamModel)
	}
	plan, err = cat.Resolve("m:alt_chat", catalog.WireOpenAIChat)
	if err != nil {
		t.Fatalf("facet: %v", err)
	}
	if plan.Targets[0].UpstreamModel != "y" {
		t.Fatalf("facet model=%q want y", plan.Targets[0].UpstreamModel)
	}
}

func TestRoundRobin(t *testing.T) {
	raw := `
providers:
  a:
    credential_profile: a
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://a
  b:
    credential_profile: b
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: http://b
models:
  m:
    modalities:
      chat:
        wire: openai-chat-completions
        strategy: round_robin
        providers:
          - provider_ref: a
            model: ma
          - provider_ref: b
            model: mb
`
	tmp := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := cat.Resolve("m", catalog.WireOpenAIChat)
	second, _ := cat.Resolve("m", catalog.WireOpenAIChat)
	if len(first.Targets) != 1 || len(second.Targets) != 1 {
		t.Fatalf("round_robin should return one target")
	}
	if first.Targets[0].ProviderRef == second.Targets[0].ProviderRef {
		t.Fatalf("round_robin should alternate: %s %s", first.Targets[0].ProviderRef, second.Targets[0].ProviderRef)
	}
}
