package bandit

type Option func(*BanditDialer)

// OnSuccess sets the onSuccess callback
func OnSuccess(onSuccess func(dialer Dialer)) Option {
	return func(dialer *BanditDialer) {
		dialer.onSuccess = onSuccess
	}
}

// OnSuccess sets the onError callback
func OnError(onError func(error)) Option {
	return func(dialer *BanditDialer) {
		dialer.onError = onError
	}
}
