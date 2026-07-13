package keyring

import (
	"errors"
	"strings"
)

// ErrForbidden is returned when gateway key scopes deny access.
var ErrForbidden = errors.New("forbidden: scope denied")

// Authorize checks whether scopes allow model + wire. Empty scopes allow all.
func Authorize(scopes []string, model, wire string) error {
	if len(scopes) == 0 {
		return nil
	}
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" || s == "*" {
			return nil
		}
		if s == "model:"+model || s == "wire:"+wire {
			return nil
		}
	}
	return ErrForbidden
}

// FilterModels returns true if scopes allow the model id (any modality wire).
func FilterModels(scopes []string, modelID string, wires []string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" || s == "*" {
			return true
		}
		if s == "model:"+modelID {
			return true
		}
		if strings.HasPrefix(s, "wire:") {
			w := strings.TrimPrefix(s, "wire:")
			for _, mw := range wires {
				if mw == w {
					return true
				}
			}
		}
	}
	return false
}