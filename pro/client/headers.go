package client

type Auth struct {
	ID       int64 `json:"userId"`
	DeviceID string
	Token    string `json:"token"`
}

// To satisfy common.AuthConfig interface, same below
func (a Auth) GetUserID() int64 {
	return a.ID
}

func (a Auth) GetDeviceID() string {
	return a.DeviceID
}

func (a Auth) GetToken() string {
	return a.Token
}
