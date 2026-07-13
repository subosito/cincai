package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestResolve_poolAdapterOverridesSurface(t *testing.T) {
	t.Parallel()
	raw := `
providers:
  dashscope:
    credential_profile: dashscope-api
    surfaces:
      video:
        protocol: openai-chat-completions
        base_url: https://dashscope-intl.aliyuncs.com/compatible-mode/v1
models:
  qwen3.5-omni-flash:
    modalities:
      video:
        wire: openai-chat-completions
        providers:
          - provider_ref: dashscope
            surface: video
            adapter: dashscope-omni
  qwen3-vl-plus:
    modalities:
      video:
        wire: openai-chat-completions
        providers:
          - provider_ref: dashscope
            surface: video
`
	tmp := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}

	omni, err := cat.ResolveWithModality("qwen3.5-omni-flash", catalog.WireOpenAIChat, "video")
	if err != nil {
		t.Fatal(err)
	}
	if omni.Targets[0].Adapter != "dashscope-omni" {
		t.Fatalf("omni adapter=%q want dashscope-omni", omni.Targets[0].Adapter)
	}
	if omni.Targets[0].Protocol != "openai-chat-completions" {
		t.Fatalf("protocol=%q", omni.Targets[0].Protocol)
	}

	vl, err := cat.ResolveWithModality("qwen3-vl-plus", catalog.WireOpenAIChat, "video")
	if err != nil {
		t.Fatal(err)
	}
	if vl.Targets[0].Adapter != "" {
		t.Fatalf("vl adapter=%q want passthrough", vl.Targets[0].Adapter)
	}
}