package client

import (
	"net/http"
	"strconv"

	"github.com/getlantern/flashlight/common"
)

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

func (u User) headers() http.Header {
	h := http.Header{}
	// auto headers
	if u.Auth.DeviceID != "" {
		h[common.DeviceIdHeader] = []string{u.Auth.DeviceID}
	}
	if u.ID != 0 {
		h[common.UserIdHeader] = []string{strconv.FormatInt(u.ID, 10)}
	}
	if u.Auth.Token != "" {
		h[common.ProTokenHeader] = []string{u.Auth.Token}
	}
	return h
}
