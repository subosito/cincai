package wiretranslate

import "github.com/subosito/cincai/pack"

func init() {
	pack.RegisterAdapter(New())
}