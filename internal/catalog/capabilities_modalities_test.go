package catalog

import (
	"testing"

	corecatalog "github.com/subosito/cincai/catalog"
	"gopkg.in/yaml.v3"
)

func documentFromRoot(t *testing.T, root map[string]any) corecatalog.Document {
	t.Helper()
	if providers, ok := root["providers"].(map[string]any); ok {
		root["providers"] = normalizeProviders(providers)
	}
	if models, ok := root["models"].(map[string]any); ok {
		root["models"] = normalizeModels(models)
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var doc corecatalog.Document
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCapabilitiesImageIsGenerateSurface(t *testing.T) {
	doc := documentFromRoot(t, map[string]any{
		"providers": map[string]any{
			"xai": map[string]any{
				"credential_profile": "xai",
				"capabilities": map[string]any{
					"image_gen": map[string]any{
						"adapter":  "xai",
						"base_url": "https://api.x.ai/v1/images",
					},
				},
			},
		},
		"models": map[string]any{
			"img": map[string]any{
				"modalities": map[string]any{
					"image_gen": map[string]any{"provider_ref": "xai"},
				},
			},
		},
	})
	surf := doc.Providers["xai"].Surfaces["image_gen"]
	if surf.Adapter != "xai" {
		t.Fatalf("surface=%+v", surf)
	}
	cat, err := corecatalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cat.Resolve("img", corecatalog.WireOpenAIImagesGen)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Targets[0].Adapter != "xai" {
		t.Fatalf("target=%+v", plan.Targets[0])
	}
}

func TestModalitiesImageDefaultsToReadWire(t *testing.T) {
	doc := documentFromRoot(t, map[string]any{
		"providers": map[string]any{
			"mimo": map[string]any{
				"credential_profile": "mimo",
				"capabilities": map[string]any{
					"chat": map[string]any{
						"protocol": "anthropic-messages",
						"base_url": "https://example.com",
					},
				},
			},
		},
		"models": map[string]any{
			"mimo-v2.5": map[string]any{
				"modalities": map[string]any{
					"image": map[string]any{"provider_ref": "mimo"},
				},
			},
		},
	})
	if doc.Models["mimo-v2.5"].Modalities["image"].Wire != "openai-chat-completions" {
		t.Fatalf("wire=%q", doc.Models["mimo-v2.5"].Modalities["image"].Wire)
	}
}

func TestImageGenDefaultsToImagesWire(t *testing.T) {
	doc := documentFromRoot(t, map[string]any{
		"providers": map[string]any{
			"xai": map[string]any{
				"credential_profile": "xai",
				"capabilities": map[string]any{
					"image_gen": map[string]any{
						"adapter":  "xai",
						"base_url": "https://api.x.ai/v1/images",
					},
				},
			},
		},
		"models": map[string]any{
			"img": map[string]any{
				"modalities": map[string]any{
					"image_gen": map[string]any{"provider_ref": "xai"},
				},
			},
		},
	})
	mod := doc.Models["img"].Modalities["image_gen"]
	if mod.Wire != "openai-images-generations" {
		t.Fatalf("wire=%q", mod.Wire)
	}
}

func TestModalitiesVoiceDefaultsToReadWire(t *testing.T) {
	doc := documentFromRoot(t, map[string]any{
		"providers": map[string]any{
			"groq": map[string]any{
				"credential_profile": "groq",
				"capabilities": map[string]any{
					"voice": map[string]any{
						"protocol": "openai-transcriptions",
						"base_url": "https://api.groq.com/openai/v1",
					},
				},
			},
		},
		"models": map[string]any{
			"whisper-large-v3-turbo": map[string]any{
				"modalities": map[string]any{
					"voice": map[string]any{"provider_ref": "groq"},
				},
			},
		},
	})
	if doc.Models["whisper-large-v3-turbo"].Modalities["voice"].Wire != "openai-audio-transcriptions" {
		t.Fatalf("wire=%q", doc.Models["whisper-large-v3-turbo"].Modalities["voice"].Wire)
	}
}
