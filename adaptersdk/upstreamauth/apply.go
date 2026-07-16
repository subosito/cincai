// Package upstreamauth resolves catalog inject overrides and applies upstream credentials.
package upstreamauth

import (
	"net/http"

	"github.com/subosito/cincai/adaptersdk/handler"
	"github.com/subosito/cincai/credential/inject"
)

// PresetBearer is the default API-key inject shape.
const PresetBearer = "bearer"

// PresetXAPIKey sends the API key in x-api-key (catalog inject_preset shorthand only).
const PresetXAPIKey = "x-api-key"

// BearerDefault is the usual adapter fallback when yaml omits inject.
func BearerDefault() inject.AdapterDefault {
	return inject.AdapterDefault{Preset: PresetBearer}
}

// Apply copies ingress headers, strips client auth, and injects upstream credentials.
// Resolution: inject: map → inject_preset (bearer | x-api-key) → adapterDefault → bearer.
func Apply(t handler.Target, req *http.Request, hdr http.Header, adapterDefault inject.AdapterDefault) error {
	inject.CopyHeaders(req, hdr)
	return inject.ApplyRoute(t.Material, req, inject.Route{
		Spec:   t.Inject,
		Preset: t.InjectPreset,
	}, adapterDefault)
}

// ApplyTranslated is for adapters that replace the ingress JSON body before upstream
// forward. Ingress Content-Type and Content-Length describe the client body, not the
// translated payload — copying them breaks strict upstream parsers (e.g. xAI images).
func ApplyTranslated(t handler.Target, req *http.Request, hdr http.Header, adapterDefault inject.AdapterDefault) error {
	if hdr != nil {
		hdr = hdr.Clone()
		hdr.Del("Content-Type")
		hdr.Del("Content-Length")
	}
	if err := Apply(t, req, hdr, adapterDefault); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("Content-Length")
	return nil
}
