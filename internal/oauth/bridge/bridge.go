package bridge

import (
	"context"
	"fmt"

	credpkg "github.com/subosito/cincai/credential"
	"github.com/subosito/cincai/credential/oauth/flow"
	oauthpack "github.com/subosito/cincai/credential/oauth/pack"
	"github.com/subosito/cincai/credential/oauth/wire"

	"github.com/subosito/cincai/credential/oauth/generic"
	"github.com/subosito/cincai/credential/store"
)

type module struct {
	login   oauthpack.LoginFn
	refresh oauthpack.RefreshFn
	device  oauthpack.LoginFn
}

func (m module) Login(ctx context.Context, profile string, oauthFlow generic.Flow, _ string, ctrl generic.Controller) (store.Material, error) {
	fctrl := toFlowCtrl(ctrl)
	var (
		payload wire.OAuthPayload
		err     error
	)
	switch oauthFlow {
	case generic.FlowDevice:
		if m.device == nil {
			return store.Material{}, fmt.Errorf("device flow not supported for profile %q", profile)
		}
		payload, err = m.device(ctx, fctrl)
	default:
		payload, err = m.login(ctx, fctrl)
	}
	if err != nil {
		return store.Material{}, err
	}
	return credpkg.MaterialFromOAuth(profile, payload), nil
}

func (m module) Refresh(ctx context.Context, profile string, cur store.Material) (store.Material, error) {
	payload := credpkg.OAuthPayloadFromMaterial(cur)
	refreshed, err := m.refresh(ctx, payload)
	if err != nil {
		return store.Material{}, err
	}
	return credpkg.MaterialFromOAuth(profile, refreshed), nil
}

// FromEntry wraps a pack registration for cincai vendor.MustRegister.
func FromEntry(e oauthpack.Entry) module {
	return module{login: e.Login, refresh: e.Refresh, device: e.Device}
}

func toFlowCtrl(ctrl generic.Controller) flow.Controller {
	out := flow.Controller{
		OnProgress: ctrl.OnProgress,
	}
	if ctrl.OnAuth != nil {
		out.OnAuth = func(info flow.AuthInfo) {
			ctrl.OnAuth(generic.AuthInfo{
				URL:          info.URL,
				Instructions: info.Instructions,
				UserCode:     info.UserCode,
			})
		}
	}
	if ctrl.OnManualInput != nil {
		out.OnManualInput = ctrl.OnManualInput
	}
	return out
}