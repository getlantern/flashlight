package common

// AuthConfig retrieves any custom info for interacting with internal services.
type AuthConfig interface {
	GetAppName() string
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
	GetEnabledExperiments() []string
}

// NullAuthConfig is useful for testing
type NullAuthConfig struct{}

func (a NullAuthConfig) GetAppName() string              { return DefaultAppName }
func (a NullAuthConfig) GetDeviceID() string             { return "" }
func (a NullAuthConfig) GetUserID() int64                { return int64(10) }
func (a NullAuthConfig) GetToken() string                { return "" }
func (a NullAuthConfig) GetEnabledExperiments() []string { return nil }

var _ AuthConfig = (*NullAuthConfig)(nil)

// NullUserConfig is useful for testing
type NullUserConfig struct{ NullAuthConfig }

func (s NullUserConfig) GetTimeZone() (string, error) {
	return "", nil
}

func (s NullUserConfig) GetLanguage() string                   { return "" }
func (s NullUserConfig) GetInternalHeaders() map[string]string { return make(map[string]string) }

var _ UserConfig = (*NullUserConfig)(nil)
