// Package pack wires Cincai handlers into compose.
// Vendor adapters live under internal/adapters/*; protocol bridges (wire-translate) under internal/wiretranslate/.
package pack

import (
	"github.com/subosito/cincai/adaptersdk"
)

var adapters []adaptersdk.Adapter

func RegisterAdapter(a adaptersdk.Adapter) {
	if a == nil {
		return
	}
	adapters = append(adapters, a)
}

func Adapters() []adaptersdk.Adapter {
	out := make([]adaptersdk.Adapter, len(adapters))
	copy(out, adapters)
	return out
}