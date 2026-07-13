package catalog_test

import (
	"testing"

	"github.com/subosito/cincai/internal/catalog"
)

func TestCopyInjectFieldsMap(t *testing.T) {
	entry := map[string]any{
		"inject_preset": "bearer",
		"inject": map[string]any{
			"Authorization": "Bearer ${access}",
			"chatgpt-account-id": "${accountId}",
		},
	}
	out := map[string]any{}
	catalog.CopyInjectFields(entry, out)
	if out["inject_preset"] != "bearer" {
		t.Fatalf("preset=%v", out["inject_preset"])
	}
	spec, ok := out["inject"].(map[string]string)
	if !ok {
		t.Fatalf("inject type=%T", out["inject"])
	}
	if spec["authorization"] != "Bearer ${access}" {
		t.Fatalf("authorization=%q", spec["authorization"])
	}
	if spec["chatgpt-account-id"] != "${accountId}" {
		t.Fatalf("account=%q", spec["chatgpt-account-id"])
	}
}