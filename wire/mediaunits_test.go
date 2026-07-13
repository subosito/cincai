package wire

import (
	"testing"

	"github.com/subosito/cincai/catalog"
)

func TestRequestUnits_imagesDefaultN(t *testing.T) {
	u, ok := requestUnits(catalog.WireOpenAIImagesGen, []byte(`{"prompt":"a cat"}`))
	if !ok || u.Units != 1 || u.Unit != "image" {
		t.Fatalf("usage = %+v ok=%v, want 1 image", u, ok)
	}
}

func TestRequestUnits_imagesN(t *testing.T) {
	u, ok := requestUnits(catalog.WireOpenAIImagesGen, []byte(`{"prompt":"a cat","n":3}`))
	if !ok || u.Units != 3 || u.Unit != "image" {
		t.Fatalf("usage = %+v ok=%v, want 3 images", u, ok)
	}
}

func TestRequestUnits_ttsChars(t *testing.T) {
	u, ok := requestUnits(catalog.WireOpenAIAudioSpeech, []byte(`{"input":"hello","voice":"x"}`))
	if !ok || u.Units != 5 || u.Unit != "character" {
		t.Fatalf("usage = %+v ok=%v, want 5 characters", u, ok)
	}
}

func TestRequestUnits_ttsCountsRunesNotBytes(t *testing.T) {
	// "héllo" is 6 bytes but 5 runes; TTS bills characters.
	u, ok := requestUnits(catalog.WireOpenAIAudioSpeech, []byte(`{"input":"héllo"}`))
	if !ok || u.Units != 5 {
		t.Fatalf("usage = %+v ok=%v, want 5 (rune count)", u, ok)
	}
}

func TestRequestUnits_emptyInputNone(t *testing.T) {
	if _, ok := requestUnits(catalog.WireOpenAIAudioSpeech, []byte(`{"voice":"x"}`)); ok {
		t.Fatal("no input → no units")
	}
}

func TestRequestUnits_nonMediaWireNone(t *testing.T) {
	if _, ok := requestUnits(catalog.WireOpenAIChat, []byte(`{"model":"m"}`)); ok {
		t.Fatal("chat wire has no media units")
	}
}
