package catalog_test

import (
	"strings"
	"testing"

	"github.com/subosito/cincai/catalog"
)

func groupsDoc() catalog.Document {
	return catalog.Document{
		Providers: map[string]catalog.Provider{
			"p": {
				CredentialProfile: "p",
				Surfaces: map[string]catalog.Surface{
					"chat": {Protocol: "openai-chat-completions", BaseURL: "https://example"},
				},
			},
		},
		Models: map[string]catalog.Model{
			"glm-5.2": {
				Modalities: map[string]catalog.Modality{
					"chat": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p", Model: "glm-5.2"}}},
				},
			},
			"gpt-5.6-luna": {
				Modalities: map[string]catalog.Modality{
					"chat": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p", Model: "gpt-5.6-luna"}}},
				},
			},
			"qwen3.8-max": {
				Modalities: map[string]catalog.Modality{
					"chat": {Wire: catalog.WireOpenAIChat, Providers: []catalog.PoolEntry{{ProviderRef: "p", Model: "qwen3.8-max"}}},
				},
			},
		},
		Groups: map[string]catalog.ModelGroup{
			"reviewers": {
				Description: "Code / PR review",
				Models:      []string{"glm-5.2", "gpt-5.6-luna", "qwen3.8-max"},
			},
		},
	}
}

func TestListModels_includesModelGroups(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(groupsDoc())
	if err != nil {
		t.Fatal(err)
	}
	list := cat.ListModels()
	var group *catalog.ModelListItem
	var luna *catalog.ModelListItem
	for i := range list.Data {
		it := &list.Data[i]
		switch it.ID {
		case "reviewers":
			group = it
		case "gpt-5.6-luna":
			luna = it
		}
	}
	if group == nil {
		t.Fatal("reviewers group missing from list")
	}
	if group.Object != catalog.ObjectModelGroup {
		t.Fatalf("object=%q", group.Object)
	}
	if group.Description != "Code / PR review" {
		t.Fatalf("description=%q", group.Description)
	}
	if strings.Join(group.Models, ",") != "glm-5.2,gpt-5.6-luna,qwen3.8-max" {
		t.Fatalf("members=%v", group.Models)
	}
	if group.Wire != "" || group.Facet != "" {
		t.Fatalf("group should not advertise wire/facet: %+v", group)
	}
	if luna == nil {
		t.Fatal("luna missing")
	}
	if luna.Object != catalog.ObjectModel {
		t.Fatalf("luna object=%q", luna.Object)
	}
	if strings.Join(luna.Groups, ",") != "reviewers" {
		t.Fatalf("luna groups=%v", luna.Groups)
	}
}

func TestValidateRoutes_groups(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(groupsDoc())
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.ValidateRoutes(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRoutes_groupUnknownMember(t *testing.T) {
	t.Parallel()
	doc := groupsDoc()
	doc.Groups["broken"] = catalog.ModelGroup{Models: []string{"nope"}}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	err = cat.ValidateRoutes()
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("want unknown member error, got %v", err)
	}
}

func TestValidateRoutes_groupCollidesWithModel(t *testing.T) {
	t.Parallel()
	doc := groupsDoc()
	doc.Groups["glm-5.2"] = catalog.ModelGroup{Models: []string{"gpt-5.6-luna"}}
	cat, err := catalog.NewFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	err = cat.ValidateRoutes()
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want collision error, got %v", err)
	}
}

func TestIsModelGroup(t *testing.T) {
	t.Parallel()
	cat, err := catalog.NewFromDocument(groupsDoc())
	if err != nil {
		t.Fatal(err)
	}
	if !cat.IsModelGroup("reviewers") {
		t.Fatal("expected reviewers to be a group")
	}
	if cat.IsModelGroup("glm-5.2") {
		t.Fatal("glm-5.2 is a model, not a group")
	}
	// Group ids are not resolvable as models.
	_, err = cat.Resolve("reviewers", catalog.WireOpenAIChat)
	if err == nil {
		t.Fatal("resolve group id should fail")
	}
}
