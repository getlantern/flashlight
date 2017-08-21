package client

type Auth struct {
	ID       int64 `json:"userId"`
	DeviceID string
	Token    string `json:"token"`
}
