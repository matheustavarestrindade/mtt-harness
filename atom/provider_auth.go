package atom

import "time"

// OAuthCredential is server-only authentication state. Public API responses must
// use ProviderConnection instead; token fields are excluded from JSON encoding.
type OAuthCredential struct {
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
	AccountID    string
	Residency    string
	ExpiresAt    time.Time
}

// ProviderConnection extends provider metadata without exposing credentials.
type ProviderConnection struct {
	ProviderSpec
	Connected  bool
	ModelCount int
}

// DeviceLogin is the public view of a device-code login. ID belongs to the
// harness; the upstream device secret and OAuth tokens never leave the server.
type DeviceLogin struct {
	ID              string
	Provider        string
	VerificationURL string
	UserCode        string
	ExpiresAt       time.Time
	Status          string
	Error           string
}
