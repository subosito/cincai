package wire

// OAuthPayload is stored credential data for type oauth.
type OAuthPayload struct {
	Type      string `json:"type"`
	Refresh   string `json:"refresh"`
	Access    string `json:"access"`
	Expires   int64  `json:"expires,omitempty"`
	Email     string `json:"email,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}
