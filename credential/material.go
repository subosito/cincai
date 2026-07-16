package credential

import (
	"strings"
	"time"

	"github.com/subosito/cincai/credential/oauth/wire"

	"github.com/subosito/cincai/credential/store"
)

// MaterialFromOAuth maps vault OAuth JSON to cincai inject material.
func MaterialFromOAuth(profile string, p wire.OAuthPayload) store.Material {
	extras := map[string]string{}
	if id := strings.TrimSpace(p.AccountID); id != "" {
		extras["account_id"] = id
	}
	if id := strings.TrimSpace(p.ProjectID); id != "" {
		extras["project_id"] = id
	}
	mat := store.Material{
		Profile:      profile,
		Kind:         store.KindOAuth,
		AccessToken:  p.Access,
		RefreshToken: p.Refresh,
		Email:        p.Email,
		Extras:       extras,
	}
	if p.Expires > 0 {
		mat.ExpiresAt = time.UnixMilli(p.Expires)
	}
	return mat
}

// OAuthPayloadFromMaterial maps cincai material back to wire OAuth JSON.
func OAuthPayloadFromMaterial(cur store.Material) wire.OAuthPayload {
	payload := wire.OAuthPayload{
		Type:      "oauth",
		Refresh:   cur.RefreshToken,
		Access:    cur.AccessToken,
		Email:     cur.Email,
		AccountID: cur.Extra("account_id"),
		ProjectID: cur.Extra("project_id"),
	}
	if !cur.ExpiresAt.IsZero() {
		payload.Expires = cur.ExpiresAt.UnixMilli()
	}
	return payload
}
