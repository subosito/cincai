package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func sampleDoc() catalog.Document {
	return catalog.Document{
		Providers: map[string]catalog.Provider{
			"p": {
				CredentialProfile: "p",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: catalog.WireOpenAIChat, BaseURL: "http://a"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"cheap-flash": {
				Modalities: map[string]catalog.Modality{
					"chat":  {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
					"image": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
			"media-only": {
				Modalities: map[string]catalog.Modality{
					"image_gen": {Wire: catalog.WireOpenAIImagesGen, Providers: []catalog.PoolEntry{{ProviderRef: "p"}}},
				},
			},
		},
	}
}

func TestMergeMeta_listsContextAndPricing(t *testing.T) {
	cat, err := catalog.NewFromDocument(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	err = cat.MergeMeta(catalog.MetaDocument{
		Billing: catalog.MetaBilling{Currency: "USD", Version: "2026-07-29"},
		Models: map[string]catalog.ModelMeta{
			"cheap-flash": {
				ContextWindow:   1_000_000,
				MaxOutputTokens: 65_536,
				Pricing: &catalog.ModelPricing{
					InputPerMTok:     0.14,
					OutputPerMTok:    0.28,
					CacheReadPerMTok: 0.0028,
				},
			},
			"media-only": {
				Pricing: &catalog.ModelPricing{PerUnit: 0.05, Unit: "image"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cat.MetaVersion() != "2026-07-29" {
		t.Fatalf("version=%q", cat.MetaVersion())
	}

	byID := map[string]catalog.ModelListItem{}
	for _, it := range cat.ListModels().Data {
		byID[it.ID] = it
	}
	base := byID["cheap-flash"]
	if base.ContextWindow != 1_000_000 || base.MaxOutputTokens != 65_536 {
		t.Fatalf("base windows: %+v", base)
	}
	if base.Pricing == nil || base.Pricing.InputPerMTok != 0.14 || base.Pricing.Currency != "USD" {
		t.Fatalf("base pricing: %+v", base.Pricing)
	}
	// Expanded facet inherits base meta.
	img := byID["cheap-flash:image"]
	if img.ContextWindow != 1_000_000 || img.Pricing == nil || img.Pricing.OutputPerMTok != 0.28 {
		t.Fatalf("facet inherit: %+v", img)
	}
	media := byID["media-only"]
	if media.Pricing == nil || media.Pricing.PerUnit != 0.05 || media.Pricing.Unit != "image" {
		t.Fatalf("media pricing: %+v", media.Pricing)
	}
}

func TestMergeMeta_unknownModelErrors(t *testing.T) {
	cat, err := catalog.NewFromDocument(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	err = cat.MergeMeta(catalog.MetaDocument{
		Models: map[string]catalog.ModelMeta{
			"no-such-model": {ContextWindow: 100},
		},
	})
	if err == nil {
		t.Fatal("expected unknown model error")
	}
}

func TestMergeMeta_emptyEntryErrors(t *testing.T) {
	cat, err := catalog.NewFromDocument(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	err = cat.MergeMeta(catalog.MetaDocument{
		Models: map[string]catalog.ModelMeta{
			"cheap-flash": {},
		},
	})
	if err == nil {
		t.Fatal("expected empty entry error")
	}
}

func TestMergeMetaFile(t *testing.T) {
	cat, err := catalog.NewFromDocument(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "models.meta.yaml")
	raw := []byte(`
billing:
  currency: USD
  version: "test"
models:
  cheap-flash:
    context_window: 262144
    pricing:
      input_per_mtok: 0.2
      output_per_mtok: 1.15
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cat.MergeMetaFile(path); err != nil {
		t.Fatal(err)
	}
	m, ok := cat.MetaFor("cheap-flash")
	if !ok || m.ContextWindow != 262144 {
		t.Fatalf("meta=%v ok=%v", m, ok)
	}
	if err := cat.MergeMetaFile(""); err != nil {
		t.Fatal(err)
	}
}
