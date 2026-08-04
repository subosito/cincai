package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/subosito/cincai/adaptersdk"
	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/catalog"
	"github.com/subosito/cincai/credential/store"
	"github.com/subosito/cincai/ingress/keyring"
	"github.com/subosito/cincai/internal/limits"
	"github.com/subosito/cincai/observability"
	"github.com/subosito/cincai/passthrough"
	"github.com/subosito/cincai/upstream"
)

// Engine handles ingress wires (chat, messages, embeddings, media, models, healthz).
type Engine struct {
	Catalog  *catalog.Catalog
	Store    store.Store
	Adapters *adaptersdk.Registry
	Auth     *keyring.Authenticator
	Client   *upstream.Client
}

func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", e.root)
	mux.HandleFunc("/v1/healthz", e.healthz)
	mux.HandleFunc("/v1/models", e.models)
	mux.HandleFunc("/v1/chat/completions", e.withAuth(catalog.WireOpenAIChat))
	mux.HandleFunc("/v1/messages/count_tokens", e.withAuth(catalog.WireAnthropicMsg))
	mux.HandleFunc("/v1/messages", e.withAuth(catalog.WireAnthropicMsg))
	mux.HandleFunc("/v1/embeddings", e.withAuth(catalog.WireOpenAIEmbed))
	mux.HandleFunc("/v1/responses", e.withAuth(catalog.WireOpenAIResponses))
	mux.HandleFunc("POST /v1/images/generations", e.withAuth(catalog.WireOpenAIImagesGen))
	mux.HandleFunc("POST /v1/images/edits", e.withAuth(catalog.WireOpenAIImagesGen))
	mux.HandleFunc("POST /v1/audio/speech", e.withAuth(catalog.WireOpenAIAudioSpeech))
	mux.HandleFunc("POST /v1/audio/transcriptions", e.withAuth(catalog.WireOpenAIAudioTranscriptions))
	mux.HandleFunc("POST /v1/videos/generations", e.withAuth(catalog.WireOpenAIVideos))
	mux.HandleFunc("POST /v1/videos/edits", e.withAuth(catalog.WireOpenAIVideos))
	mux.HandleFunc("POST /v1/videos/extensions", e.withAuth(catalog.WireOpenAIVideos))
	mux.HandleFunc("GET /v1/videos/{id}", e.withAuth(catalog.WireOpenAIVideos))
	return observability.IngressLog("", mux)
}

func setWire(rec *observability.Recorder, wireID string) {
	if rec != nil {
		rec.Wire = wireID
	}
}

func (e *Engine) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		setWire(observability.RecorderFrom(r.Context()), "root")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (e *Engine) healthz(w http.ResponseWriter, r *http.Request) {
	setWire(observability.RecorderFrom(r.Context()), "healthz")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (e *Engine) models(w http.ResponseWriter, r *http.Request) {
	setWire(observability.RecorderFrom(r.Context()), "models")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, err := e.Auth.Authenticate(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if rec := observability.RecorderFrom(r.Context()); rec != nil {
		rec.PrincipalID = p.ID
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(e.Catalog.ListModelsFor(p.Scopes))
}

func (e *Engine) withAuth(wireID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setWire(observability.RecorderFrom(r.Context()), wireID)
		p, err := e.Auth.Authenticate(r.Context(), r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if rec := observability.RecorderFrom(r.Context()); rec != nil {
			rec.PrincipalID = p.ID
		}
		// Pre-request rolling token budget (scopes still apply inside handleWire).
		if e.Auth != nil && e.Auth.Store != nil && p.HasBudget() {
			st, err := keyring.CheckBudget(r.Context(), e.Auth.Store, p)
			if err != nil {
				if errors.Is(err, keyring.ErrBudgetExceeded) {
					keyring.WriteBudgetExceeded(w, st)
					return
				}
				slog.ErrorContext(r.Context(), "budget check", "principal", p.ID, "err", err)
				http.Error(w, "budget check failed", http.StatusInternalServerError)
				return
			}
		}
		e.handleWire(w, r, p, wireID)
		// Post-request: count measured tokens toward the rolling ledger.
		if e.Auth != nil && e.Auth.Store != nil && p.HasBudget() {
			if rec := observability.RecorderFrom(r.Context()); rec != nil {
				if err := keyring.RecordBudgetTokens(r.Context(), e.Auth.Store, p, rec.Usage.InputTokens, rec.Usage.OutputTokens); err != nil {
					slog.ErrorContext(r.Context(), "budget record", "principal", p.ID, "err", err)
				}
			}
		}
	}
}

type modelBody struct {
	Model string `json:"model"`
}

func (e *Engine) readModel(r *http.Request) (string, []byte, error) {
	raw, err := readLimitedBody(r.Body, limits.MaxRequestBody)
	r.Body.Close()
	if err != nil {
		return "", nil, err
	}
	var mb modelBody
	_ = json.Unmarshal(raw, &mb)
	return strings.TrimSpace(mb.Model), raw, nil
}

func modelFromMultipart(raw []byte, contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return "", nil
	}
	mr := multipart.NewReader(bytes.NewReader(raw), boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if part.FormName() != "model" {
			continue
		}
		b, err := io.ReadAll(part)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", nil
}

func (e *Engine) readIngress(r *http.Request, wireID string) (model string, raw []byte, err error) {
	if wireID == catalog.WireOpenAIVideos && r.Method == http.MethodGet {
		model = strings.TrimSpace(r.URL.Query().Get("model"))
		return model, nil, nil
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		raw, err := readLimitedBody(r.Body, limits.MaxRequestBody)
		r.Body.Close()
		if err != nil {
			return "", nil, err
		}
		model, err = modelFromMultipart(raw, ct)
		if err != nil {
			return "", nil, err
		}
		return model, raw, nil
	}
	return e.readModel(r)
}

func rewriteModelBody(raw []byte, upstreamModel string) []byte {
	if upstreamModel == "" {
		return raw
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	modelJSON, err := json.Marshal(upstreamModel)
	if err != nil {
		return raw
	}
	m["model"] = modelJSON
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

func readLimitedBody(body io.Reader, max int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > max {
		return nil, fmt.Errorf("request body too large")
	}
	return raw, nil
}

func refreshContentLength(r *http.Request, n int) {
	r.ContentLength = int64(n)
	r.Header.Set("Content-Length", strconv.Itoa(n))
}

func toHandlerTarget(t catalog.Target, m store.Material) handler.Target {
	return handler.Target{Target: t, Material: m}
}

func recordTarget(rec *observability.Recorder, target catalog.Target) {
	if rec == nil {
		return
	}
	rec.ProviderRef = target.ProviderRef
	if target.Adapter != "" {
		rec.Protocol = "adapter:" + target.Adapter
	} else {
		rec.Protocol = target.Protocol
	}
}

func (e *Engine) requestBody(raw []byte, upstreamModel string, r *http.Request) io.ReadCloser {
	if raw == nil {
		return nil
	}
	rewritten := rewriteModelBody(raw, upstreamModel)
	refreshContentLength(r, len(rewritten))
	return io.NopCloser(bytes.NewReader(rewritten))
}

// forceRefresher is implemented by credential stores that can eagerly refresh an
// OAuth profile (credential/refresh.Store). When the store does not implement it,
// reactive 401 retry is skipped.
type forceRefresher interface {
	ForceRefresh(ctx context.Context, profile string, prev store.Material) (store.Material, error)
}

// forwardTarget sends one attempt to target. On a 401 it refreshes the target's
// OAuth credential once and retries, covering an access token that expired
// between the proactive refresh check and the upstream call.
func (e *Engine) forwardTarget(ctx context.Context, wireID, ingressPath string, target catalog.Target, mat store.Material, raw []byte, r *http.Request, hdr http.Header) (*http.Response, error) {
	resp, err := e.forward(ctx, wireID, ingressPath, toHandlerTarget(target, mat), e.requestBody(raw, target.UpstreamModel, r), hdr)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized || target.CredentialProfile == "" {
		return resp, nil
	}
	fr, ok := e.Store.(forceRefresher)
	if !ok {
		return resp, nil
	}
	newMat, rerr := fr.ForceRefresh(ctx, target.CredentialProfile, mat)
	if rerr != nil || newMat.AccessToken == "" || newMat.AccessToken == mat.AccessToken {
		return resp, nil // no fresh token to retry with — keep the 401
	}
	resp.Body.Close()
	return e.forward(ctx, wireID, ingressPath, toHandlerTarget(target, newMat), e.requestBody(raw, target.UpstreamModel, r), hdr)
}

func (e *Engine) forward(ctx context.Context, wireID, ingressPath string, ht handler.Target, body io.ReadCloser, hdr http.Header) (*http.Response, error) {
	target := ht.Target
	httpClient := e.httpClient(target.Proxy)
	switch wireID {
	case catalog.WireOpenAIEmbed:
		h, ok := lookupEmbed(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		return h.Forward(ctx, httpClient, ht, body, hdr)
	case catalog.WireOpenAIImagesGen:
		h, ok := lookupImage(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		return h.Forward(ctx, httpClient, ht, ingressPath, body, hdr)
	case catalog.WireOpenAIAudioSpeech:
		h, ok := lookupSpeech(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		return h.Forward(ctx, httpClient, ht, body, hdr)
	case catalog.WireOpenAIAudioTranscriptions:
		h, ok := lookupTranscription(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		return h.Forward(ctx, httpClient, ht, body, hdr)
	case catalog.WireOpenAIVideos:
		h, ok := lookupVideo(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		return h.Forward(ctx, httpClient, ht, ingressPath, body, hdr)
	default:
		h, ok := lookupChat(e.Adapters, target)
		if !ok {
			return nil, errRouteNotRegistered
		}
		if wireID == catalog.WireAnthropicMsg && ingressPath != "" && ingressPath != "/v1/messages" {
			return passthrough.RelayPath(ctx, httpClient, ht, ingressPath, body, hdr)
		}
		return h.Forward(ctx, httpClient, ht, body, hdr)
	}
}

// httpClient picks the outbound client for a provider proxy setting.
// Empty proxy uses Engine.Client when set (tests inject custom clients), else ClientFor("").
func (e *Engine) httpClient(proxy string) *http.Client {
	p := strings.TrimSpace(proxy)
	if p == "" && e.Client != nil && e.Client.HTTP != nil {
		return e.Client.HTTP
	}
	return upstream.ClientFor(p).HTTP
}

func (e *Engine) handleWire(w http.ResponseWriter, r *http.Request, p keyring.Principal, wireID string) {
	if err := validateWireMethod(r, wireID); err != nil {
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return
	}
	rec := observability.RecorderFrom(r.Context())
	model, raw, err := e.readIngress(r, wireID)
	if err != nil || model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	raw, injectedUsage := injectIncludeUsage(wireID, raw)
	if u, ok := requestUnits(wireID, raw); ok && rec != nil {
		rec.Usage = u // a cincai adapter may override this during Forward
	}
	if rec != nil {
		rec.Model = model
	}
	if err := keyring.Authorize(p.Scopes, model, wireID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// Routing is model id + wire only. Same-wire multi-modality expands to
	// base:facet public ids at catalog load (no custom client headers).
	// groups: entries are discovery-only — not valid request model ids.
	if e.Catalog.IsModelGroup(model) {
		http.Error(w, fmt.Sprintf("model %q is a model_group (not callable; pick a member from GET /v1/models)", model), http.StatusBadRequest)
		return
	}
	plan, err := e.Catalog.ResolveWithModality(model, wireID, catalog.DefaultModalityForWire(wireID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Composite request: log hop as model, requested id as alias.
	composite := len(plan.Models) > 0
	if rec != nil && composite {
		rec.Alias = model
	}
	// Effort is applied per target hop: composite models (modality.models) map to
	// different public ids, each with its own efforts list. Client body effort is
	// the hint; hop ApplyEffort validates / rewrites SKU + ExpandEffortBody.
	effortHint := ""
	if isChatWire(wireID) && raw != nil {
		effortHint = catalog.EffortFromBody(raw)
	}
	failover := plan.Strategy == catalog.StrategyFailover || composite
	ingressPath := r.URL.Path
	ctx := r.Context()
	outHdr := r.Header.Clone()

	for i, target := range plan.Targets {
		hopRaw := raw
		hopID := target.Model
		if hopID == "" {
			hopID = model
		}
		if rec != nil && composite {
			// Real catalog hop (not the composite id) for usage / cost tracking.
			rec.Model = hopID
		}
		if isChatWire(wireID) && raw != nil {
			// Effort for the hop public id (target.Model), not the request alias.
			tgs := []catalog.Target{target}
			used, err := e.Catalog.ApplyEffort(hopID, effortHint, tgs)
			if err != nil {
				// Unsupported effort on this hop — try next if composite/failover.
				if failover && i < len(plan.Targets)-1 {
					slog.InfoContext(ctx, "effort skip hop", "requested", model, "hop", hopID, "err", err.Error())
					continue
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			target = tgs[0]
			if used != "" {
				if m, ok := e.Catalog.Model(hopID); ok {
					hopRaw, err = catalog.ExpandEffortBody(wireID, raw, used, m)
					if err != nil {
						if failover && i < len(plan.Targets)-1 {
							continue
						}
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}
			}
		}
		recordTarget(rec, target)
		mat, err := e.Store.Get(ctx, target.CredentialProfile)
		if err != nil {
			slog.ErrorContext(ctx, "upstream credential", "profile", target.CredentialProfile, "provider_ref", target.ProviderRef, "err", err)
			if failover && i < len(plan.Targets)-1 {
				continue
			}
			http.Error(w, "upstream credential unavailable", http.StatusBadGateway)
			return
		}
		resp, err := e.forwardTarget(ctx, wireID, ingressPath, target, mat, hopRaw, r, outHdr)
		if err != nil {
			slog.ErrorContext(ctx, "upstream forward", "provider_ref", target.ProviderRef, "model", target.UpstreamModel, "requested", model, "hop", target.Model, "wire", wireID, "err", err)
			// errRouteNotRegistered happens before anything is sent (safe to fail over);
			// otherwise only connection-setup failures are retryable so a non-idempotent
			// POST is not executed twice.
			if failover && i < len(plan.Targets)-1 && (errors.Is(err, errRouteNotRegistered) || upstream.Retryable(0, err)) {
				continue
			}
			if errors.Is(err, errRouteNotRegistered) {
				http.Error(w, "route not registered", http.StatusBadGateway)
				return
			}
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		if failover && upstream.Retryable(resp.StatusCode, nil) && i < len(plan.Targets)-1 {
			resp.Body.Close()
			if len(plan.Models) > 0 {
				slog.InfoContext(ctx, "models chain hop fail", "requested", model, "hop", target.Model, "status", resp.StatusCode, "next", true)
			}
			continue
		}
		// CopyResponse / CopyResponseWithUsage own and close resp.Body.
		// Adapters must not Close a body they hand back for streaming; async
		// pipes (wire-translate) keep the upstream body open until drained.
		if meter := usageMeterFor(wireID); meter != nil {
			meter.encoding = resp.Header.Get("Content-Encoding")
			var drop func([]byte) bool
			if injectedUsage {
				drop = isUsageOnlyDataLine
			}
			_ = upstream.CopyResponseWithUsage(ctx, w, resp, meter, drop)
			if rec != nil {
				rec.Usage = meter.Result()
			}
		} else {
			_ = upstream.CopyResponse(ctx, w, resp)
		}
		return
	}
}

func isChatWire(wireID string) bool {
	switch wireID {
	case catalog.WireOpenAIChat, catalog.WireOpenAIResponses, catalog.WireAnthropicMsg:
		return true
	default:
		return false
	}
}

func validateWireMethod(r *http.Request, wireID string) error {
	switch wireID {
	case catalog.WireOpenAIImagesGen, catalog.WireOpenAIAudioSpeech, catalog.WireOpenAIAudioTranscriptions:
		if r.Method != http.MethodPost {
			return errMethodNotAllowed
		}
	case catalog.WireOpenAIVideos:
		switch r.URL.Path {
		case "/v1/videos/generations", "/v1/videos/edits", "/v1/videos/extensions":
			if r.Method != http.MethodPost {
				return errMethodNotAllowed
			}
		default:
			if r.Method != http.MethodGet {
				return errMethodNotAllowed
			}
		}
	}
	return nil
}

var (
	errMethodNotAllowed   = errors.New("method not allowed")
	errRouteNotRegistered = errors.New("route not registered")
)
