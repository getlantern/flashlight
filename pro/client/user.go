package client

type Device struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

type Auth struct {
	ID    int64  `json:"userId"`
	Token string `json:"token"`
}

type User struct {
	Auth           `json:",inline"`
	Email          string   `json:"email"`
	PhoneNumber    string   `json:"telephone"`
	UserStatus     string   `json:"userStatus"`
	Locale         string   `json:"locale"`
	Expiration     int64    `json:"expiration"`
	Devices        []Device `json:"devices"`
	Code           string   `json:"code"`
	ExpireAt       int64    `json:"expireAt"`
	Referral       string   `json:"referral"`
	YinbiEnabled   bool     `json:"yinbiEnabled"`
	RelpicaEnabled bool     `json:"replicaEnabled"`
}
