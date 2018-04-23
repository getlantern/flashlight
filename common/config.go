// a struct implementation of common.UserConfig interface

package common

// an implementation of common.UserConfig
type UserConfigData struct {
	DeviceID string
	UserID   int64
	Token    string
	Headers  map[string]string
}

func (uc *UserConfigData) GetDeviceID() string { return uc.DeviceID }
func (uc *UserConfigData) GetUserID() int64    { return uc.UserID }
func (uc *UserConfigData) GetToken() string    { return uc.Token }
func (uc *UserConfigData) GetInternalHeaders() map[string]string {
	h := make(map[string]string)
	for k, v := range uc.Headers {
		h[k] = v
	}
	return h
}

var _ UserConfig = (*UserConfigData)(nil)

// Constucts a new UserConfigData (common.UserConfig) with the given options.
func NewUserConfigData(deviceID string, userID int64, token string, headers map[string]string) *UserConfigData {
	uc := &UserConfigData{
		DeviceID: deviceID,
		UserID:   userID,
		Token:    token,
		Headers:  make(map[string]string),
	}
	for k, v := range headers {
		uc.Headers[k] = v
	}
	return uc
}
