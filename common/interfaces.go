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
	GetLanguage() string
	GetTimeZone() (string, error)
	GetInternalHeaders() map[string]string
}

// NullAuthConfig is useful for testing
type NullAuthConfig struct{}

func (a NullAuthConfig) GetDeviceID() string { return "" }
func (a NullAuthConfig) GetUserID() int64    { return int64(10) }
func (a NullAuthConfig) GetToken() string    { return "" }

var _ AuthConfig = (*NullAuthConfig)(nil)

// NullUserConfig is useful for testing
type NullUserConfig struct{ NullAuthConfig }

func (s NullUserConfig) GetTimeZone() (string, error) {
	panic("implement me")
}

func (s NullUserConfig) GetLanguage() string                   { return "" }
func (s NullUserConfig) GetInternalHeaders() map[string]string { return make(map[string]string) }

var _ UserConfig = (*NullUserConfig)(nil)
