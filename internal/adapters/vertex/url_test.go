package vertex

import "testing"

func TestGenerateContentURL(t *testing.T) {
	t.Parallel()
	got, err := generateContentURL("https://aiplatform.googleapis.com/v1/projects/demo/locations/us-central1", "google/gemini-3.6-flash", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://aiplatform.googleapis.com/v1/projects/demo/locations/us-central1/publishers/google/models/gemini-3.6-flash:generateContent"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gotS, err := generateContentURL("https://example.com/v1", "google/gemini-3.5-flash", true)
	if err != nil {
		t.Fatal(err)
	}
	wantS := "https://example.com/v1/publishers/google/models/gemini-3.5-flash:streamGenerateContent?alt=sse"
	if gotS != wantS {
		t.Fatalf("stream got %q want %q", gotS, wantS)
	}
}

func TestSplitModel(t *testing.T) {
	t.Parallel()
	p, n := splitModel("google/gemini-3.6-flash")
	if p != "google" || n != "gemini-3.6-flash" {
		t.Fatalf("%s %s", p, n)
	}
	p, n = splitModel("gemini-3.1-pro")
	if p != "google" || n != "gemini-3.1-pro" {
		t.Fatalf("%s %s", p, n)
	}
}
