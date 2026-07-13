package flow

import "context"

// AuthInfo is shown to the operator during OAuth login.
type AuthInfo struct {
	URL          string
	Instructions string
	UserCode     string
}

// Controller receives OAuth flow progress callbacks.
type Controller struct {
	OnAuth        func(AuthInfo)
	OnProgress    func(string)
	OnManualInput func(context.Context) (string, error)
}