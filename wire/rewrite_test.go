package wire

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRewriteModelBodyPreservesNumbers(t *testing.T) {
	raw := []byte(`{"model":"ingress","seed":9007199254740993,"max_tokens":128}`)
	out := rewriteModelBody(raw, "upstream-model")
	if !strings.Contains(string(out), `"seed":9007199254740993`) {
		t.Fatalf("seed precision lost: %s", out)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	var model string
	if err := json.Unmarshal(m["model"], &model); err != nil {
		t.Fatal(err)
	}
	if model != "upstream-model" {
		t.Fatalf("model=%q", model)
	}
}