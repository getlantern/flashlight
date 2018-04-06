package common

// AuthConfig retrieves any custom info for interacting with internal services.
type AuthConfig interface {
	GetDeviceID() string
	GetUserID() int64
	GetToken() string
}

// UserConfig retrieves auth and other metadata passed to internal services.
type UserConfig interface {
	AuthConfig
	GetInternalHeaders() map[string]string
}
