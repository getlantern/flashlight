package client

type Device struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

type User struct {
	Email         string   `json:"email"`
	PhoneNumber   string   `json:"telephone"`
	UserStatus    string   `json:"userStatus"`
	Locale        string   `json:"locale"`
	Expiration    int64    `json:"expiration"`
	AutoconfToken string   `json:"autoconfToken"`
	Subscription  string   `json:"subscription"`
	Devices       []Device `json:"devices"`
	Code          string   `json:"code"`
	ExpireAt      int64    `json:"expireAt"`
	Referral      string   `json:"referral"`
	Auth          `json:",inline"`
}

type UserId struct {
	UserId string `json:"userId,inline"`
}

type Auth struct {
	ID    int64  `json:"userId"`
	Token string `json:"token"`
}
