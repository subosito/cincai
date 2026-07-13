package xai

import "github.com/subosito/cincai/pack"

func init() {
	pack.RegisterAdapter(NewImage())
}