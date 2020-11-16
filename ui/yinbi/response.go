package yinbi

import (
	"github.com/getlantern/auth-server/api"
)

type ImportWalletResponse struct {
	api.ApiResponse
	Username string `json:"username"`
	Address  string `json:"address"`
	Salt     string `json:"salt"`
	Seed     string `json:"seed"`
}

type RedeemCodesResponse struct {
	api.ApiResponse
	AmountAwarded int `json:"amountAwarded"`
}
