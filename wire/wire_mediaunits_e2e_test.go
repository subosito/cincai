package wire_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIngressRecordCarriesMediaUnits drives an image request with n=2 and
// asserts the ingress record carries units=2 unit=image (counted from the
// request, so it works for passthrough with no adapter).
func TestIngressRecordCarriesMediaUnits(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGk="},{"b64_json":"aGk="}]}`)
	}))
	defer up.Close()

	engine := mediaTestEngine(t, up.URL, `
providers:
  img:
    credential_profile: img
    surfaces:
      image_gen:
        protocol: openai-images
        base_url: `+up.URL+`
models:
  gpt-image-test:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: img
            model: gpt-image-2
`)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	body := `{"model":"gpt-image-test","prompt":"a cat","n":2}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/images/generations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mediaTestSecret(t, engine))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.Contains(line, `"ingress"`) {
			continue
		}
		var entry struct {
			Record struct {
				Model string `json:"model"`
				Usage *struct {
					Units int    `json:"units"`
					Unit  string `json:"unit"`
				} `json:"usage"`
			} `json:"record"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Record.Model != "gpt-image-test" {
			continue
		}
		found = true
		if entry.Record.Usage == nil || entry.Record.Usage.Units != 2 || entry.Record.Usage.Unit != "image" {
			t.Fatalf("media usage = %+v, want units=2 unit=image", entry.Record.Usage)
		}
	}
	if !found {
		t.Fatalf("no ingress record for gpt-image-test in:\n%s", buf.String())
	}
}
