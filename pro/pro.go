package pro

import "github.com/getlantern/flashlight/common"

var authConfig common.AuthConfig

func Init(ac common.AuthConfig) {
	authConfig = ac
}
