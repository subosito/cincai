// Package catalog resolves multi-surface providers.yaml to upstream targets.
package catalog

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Wire IDs.
const (
	WireOpenAIChat                = "openai-chat-completions"
	WireAnthropicMsg              = "anthropic-messages"
	WireOpenAIEmbed               = "openai-embeddings"
	WireOpenAIResponses           = "openai-responses"
	WireOpenAIImagesGen           = "openai-images-generations"
	WireOpenAIAudioSpeech         = "openai-audio-speech"
	WireOpenAIAudioTranscriptions = "openai-audio-transcriptions"
	WireOpenAIVideos              = "openai-videos"
)

// Strategy names.
const (
	StrategyFailover   = "failover"
	StrategyRoundRobin = "round_robin"
)

// Surface is one upstream endpoint on a provider (passthrough protocol or translate adapter).
type Surface struct {
	Protocol string `yaml:"protocol,omitempty"`
	Adapter  string `yaml:"adapter,omitempty"`
	BaseURL  string `yaml:"base_url"`
}

// Provider is a logical vendor connection.
type Provider struct {
	CredentialProfile string             `yaml:"credential_profile"`
	Inject            map[string]string  `yaml:"inject,omitempty"`
	InjectPreset      string             `yaml:"inject_preset,omitempty"`
	Surfaces          map[string]Surface `yaml:"surfaces"`
}

// PoolEntry is one upstream in a model pool.
type PoolEntry struct {
	ProviderRef string `yaml:"provider_ref"`
	Model       string `yaml:"model"`
	Surface     string `yaml:"surface,omitempty"`
	// Adapter overrides the provider surface handler (e.g. dashscope-omni on surface: video).
	Adapter string `yaml:"adapter,omitempty"`
}

// Modality binds wire + pool.
type Modality struct {
	Wire      string      `yaml:"wire"`
	Strategy  string      `yaml:"strategy,omitempty"`
	Providers []PoolEntry `yaml:"providers"`
}

// Model is a catalog id.
type Model struct {
	Modalities map[string]Modality `yaml:"modalities"`
}

// Document is providers.yaml root.
type Document struct {
	Providers map[string]Provider `yaml:"providers"`
	Models    map[string]Model    `yaml:"models"`
}

// Catalog is loaded operator config.
type Catalog struct {
	doc Document
	rr  map[string]*roundRobinState
	mu  sync.Mutex
	// loadedAt backs ModelListItem.Created. See listModels for why.
	loadedAt int64
}

type roundRobinState struct {
	idx int
}

// Load reads providers.yaml.
func Load(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return NewFromDocument(doc)
}

// NewFromDocument builds a catalog from an in-memory document (after shim normalization).
// Same-wire multi-modality rows are expanded to base:facet model ids (see ExpandWireCollisions).
func NewFromDocument(doc Document) (*Catalog, error) {
	if err := ExpandWireCollisions(&doc); err != nil {
		return nil, err
	}
	return &Catalog{doc: doc, rr: make(map[string]*roundRobinState), loadedAt: time.Now().Unix()}, nil
}

// WireForPath maps HTTP path to wire id.
func WireForPath(path string) (string, bool) {
	switch strings.TrimSpace(path) {
	case "/v1/chat/completions":
		return WireOpenAIChat, true
	case "/v1/messages", "/v1/messages/count_tokens":
		return WireAnthropicMsg, true
	case "/v1/embeddings":
		return WireOpenAIEmbed, true
	case "/v1/responses":
		return WireOpenAIResponses, true
	case "/v1/images/generations", "/v1/images/edits":
		return WireOpenAIImagesGen, true
	case "/v1/audio/speech":
		return WireOpenAIAudioSpeech, true
	case "/v1/audio/transcriptions":
		return WireOpenAIAudioTranscriptions, true
	case "/v1/videos/generations":
		return WireOpenAIVideos, true
	default:
		if strings.HasPrefix(path, "/v1/videos/") && path != "/v1/videos/generations" {
			return WireOpenAIVideos, true
		}
		return "", false
	}
}

// Target is resolved upstream route.
type Target struct {
	Model             string
	ProviderRef       string
	Protocol          string
	Adapter           string
	BaseURL           string
	CredentialProfile string
	UpstreamModel     string
	Inject            map[string]string
	InjectPreset      string
}

// RoutePlan is the ordered upstream attempt list for one model + wire.
type RoutePlan struct {
	Strategy string
	Targets  []Target
}

// Resolve picks upstream route(s) for model + wire.
// Failover returns all pool entries in yaml order; round_robin returns one entry.
func (c *Catalog) Resolve(model, wire string) (RoutePlan, error) {
	return c.ResolveWithModality(model, wire, "")
}

// ResolveWithModality picks upstream route(s) for model + wire, optionally constrained
// by catalog modality key (models.<id>.modalities.<key>). Empty modality auto-selects
// when unambiguous. Same-wire multi-modality should be expanded to base:facet ids at
// load (ExpandWireCollisions); residual collisions error with that guidance.
func (c *Catalog) ResolveWithModality(model, wire, modality string) (RoutePlan, error) {
	m, ok := c.doc.Models[model]
	if !ok {
		return RoutePlan{}, fmt.Errorf("unknown model %q", model)
	}
	var modNames []string
	for name, md := range m.Modalities {
		if md.Wire == wire {
			modNames = append(modNames, name)
		}
	}
	sort.Strings(modNames)
	if len(modNames) == 0 {
		return RoutePlan{}, fmt.Errorf("model %q has no modality for wire %q", model, wire)
	}
	modName, err := pickModality(modNames, modality)
	if err != nil {
		return RoutePlan{}, err
	}
	mod := m.Modalities[modName]
	if len(mod.Providers) == 0 {
		return RoutePlan{}, fmt.Errorf("model %q: empty provider pool", model)
	}
	strat := mod.Strategy
	if strat == "" {
		strat = StrategyFailover
	}
	if strat == "sticky" {
		return RoutePlan{}, fmt.Errorf("strategy %q removed; use %q or %q", strat, StrategyFailover, StrategyRoundRobin)
	}
	poolKey := modName + "|" + wire
	entries, err := c.pickEntries(poolKey, strat, mod.Providers)
	if err != nil {
		return RoutePlan{}, err
	}
	targets := make([]Target, 0, len(entries))
	for _, entry := range entries {
		t, err := c.targetFromEntry(model, entry, wire)
		if err != nil {
			return RoutePlan{}, err
		}
		targets = append(targets, t)
	}
	return RoutePlan{Strategy: strat, Targets: targets}, nil
}

func pickModality(candidates []string, hint string) (string, error) {
	hint = strings.TrimSpace(hint)
	if hint != "" {
		for _, name := range candidates {
			if name == hint {
				return name, nil
			}
		}
		return "", fmt.Errorf("model modality %q not available for wire", hint)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	// Operator-defined keys search_web/search_x: default chat when they are the only siblings.
	if hasModality(candidates, "chat") && onlySearchSiblings(candidates) {
		return "chat", nil
	}
	return "", fmt.Errorf("multiple modalities for wire: %s (use distinct model ids base%sfacet; catalog expand should have split these at load)", strings.Join(candidates, ", "), FacetSeparator)
}

func hasModality(candidates []string, name string) bool {
	for _, c := range candidates {
		if c == name {
			return true
		}
	}
	return false
}

func onlySearchSiblings(candidates []string) bool {
	for _, name := range candidates {
		if name == "chat" {
			continue
		}
		if name != "search_web" && name != "search_x" {
			return false
		}
	}
	return true
}

func (c *Catalog) targetFromEntry(model string, entry PoolEntry, wire string) (Target, error) {
	prov, ok := c.doc.Providers[entry.ProviderRef]
	if !ok {
		return Target{}, fmt.Errorf("unknown provider_ref %q", entry.ProviderRef)
	}
	surf, err := c.surfaceFor(prov, entry, wire)
	if err != nil {
		return Target{}, err
	}
	upstreamModel := strings.TrimSpace(entry.Model)
	if upstreamModel == "" {
		upstreamModel = strings.TrimSpace(model)
	}
	adapter := surf.Adapter
	if entry.Adapter != "" {
		adapter = entry.Adapter
	}
	return Target{
		Model: model, ProviderRef: entry.ProviderRef,
		Protocol: surf.Protocol, Adapter: adapter, BaseURL: surf.BaseURL,
		CredentialProfile: prov.CredentialProfile,
		UpstreamModel:     upstreamModel,
		Inject:            prov.Inject,
		InjectPreset:      prov.InjectPreset,
	}, nil
}

func (c *Catalog) surfaceFor(prov Provider, entry PoolEntry, wire string) (Surface, error) {
	if entry.Surface != "" {
		s, ok := prov.Surfaces[entry.Surface]
		if !ok {
			return Surface{}, fmt.Errorf("surface %q not found on provider", entry.Surface)
		}
		return s, nil
	}
	ids := make([]string, 0, len(prov.Surfaces))
	for id := range prov.Surfaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := prov.Surfaces[id]
		if s.Adapter != "" {
			continue
		}
		if protocolMatchesWire(s.Protocol, wire) {
			return s, nil
		}
	}
	return Surface{}, fmt.Errorf("no surface for wire %q", wire)
}

func protocolMatchesWire(protocol, wire string) bool {
	// Core matches OpenAI + Anthropic ingress wires only. Vendor protocols
	// require an explicit pool surface in providers.yaml; translate adapters
	// use surface.adapter instead of vendor protocol names.
	switch wire {
	case WireOpenAIChat:
		return protocol == "openai-chat-completions" || protocol == "openai-compat-chat"
	case WireAnthropicMsg:
		return protocol == "anthropic-messages"
	case WireOpenAIEmbed:
		return protocol == "openai-embeddings"
	case WireOpenAIResponses:
		return protocol == "openai-responses"
	case WireOpenAIImagesGen:
		return protocol == "openai-images"
	case WireOpenAIAudioSpeech:
		return protocol == "openai-tts"
	case WireOpenAIAudioTranscriptions:
		return protocol == "openai-transcriptions"
	case WireOpenAIVideos:
		return protocol == "openai-videos"
	default:
		return false
	}
}

func (c *Catalog) pickEntries(poolKey, strat string, pool []PoolEntry) ([]PoolEntry, error) {
	if len(pool) == 0 {
		return nil, fmt.Errorf("empty provider pool")
	}
	switch strat {
	case StrategyFailover:
		return pool, nil
	case StrategyRoundRobin:
		c.mu.Lock()
		defer c.mu.Unlock()
		st := c.rr[poolKey]
		if st == nil {
			st = &roundRobinState{}
			c.rr[poolKey] = st
		}
		i := st.idx % len(pool)
		st.idx++
		return []PoolEntry{pool[i]}, nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", strat)
	}
}

// RegisteredRoutes returns protocol and adapter names referenced in catalog.
func (c *Catalog) RegisteredRoutes() (protocols, adapters []string) {
	seenP := map[string]bool{}
	seenA := map[string]bool{}
	for _, p := range c.doc.Providers {
		for _, s := range p.Surfaces {
			if s.Adapter != "" {
				seenA[s.Adapter] = true
			}
			if s.Protocol != "" {
				seenP[s.Protocol] = true
			}
		}
	}
	for _, m := range c.doc.Models {
		for _, md := range m.Modalities {
			for _, entry := range md.Providers {
				if entry.Adapter != "" {
					seenA[entry.Adapter] = true
				}
			}
		}
	}
	for k := range seenP {
		protocols = append(protocols, k)
	}
	for k := range seenA {
		adapters = append(adapters, k)
	}
	return protocols, adapters
}

// Doctor checks catalog routes against registered handler sets.
func Doctor(cat *Catalog, registeredProtocols, registeredAdapters map[string]bool) []string {
	var missing []string
	protos, adpts := cat.RegisteredRoutes()
	for _, p := range protos {
		if !registeredProtocols[p] {
			missing = append(missing, "protocol:"+p)
		}
	}
	for _, a := range adpts {
		if !registeredAdapters[a] {
			missing = append(missing, "adapter:"+a)
		}
	}
	return missing
}
