package wire

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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

func TestRewriteMultipartModel(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "gpt-transcribe")
	_ = w.WriteField("language", "id")
	fw, _ := w.CreateFormFile("file", "note.ogg")
	_, _ = io.WriteString(fw, "oggbytes")
	_ = w.Close()
	ct := w.FormDataContentType()

	out, newCT, ok := rewriteMultipartModel(buf.Bytes(), ct, "openai/gpt-transcribe")
	if !ok {
		t.Fatal("expected rewrite")
	}
	if !strings.HasPrefix(newCT, "multipart/") {
		t.Fatalf("content-type=%q", newCT)
	}
	_, params, err := mime.ParseMediaType(newCT)
	if err != nil {
		t.Fatal(err)
	}
	r := multipart.NewReader(bytes.NewReader(out), params["boundary"])
	fields := map[string]string{}
	var fileName, fileData string
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(part)
		_ = part.Close()
		if part.FormName() == "file" || part.FileName() != "" {
			fileName = part.FileName()
			fileData = string(data)
			continue
		}
		fields[part.FormName()] = string(data)
	}
	if fields["model"] != "openai/gpt-transcribe" {
		t.Fatalf("model=%q fields=%v", fields["model"], fields)
	}
	if fields["language"] != "id" {
		t.Fatalf("language=%q", fields["language"])
	}
	if fileName != "note.ogg" || fileData != "oggbytes" {
		t.Fatalf("file name=%q data=%q", fileName, fileData)
	}
}
