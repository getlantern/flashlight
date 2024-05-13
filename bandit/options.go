package bandit

type Option func(*BanditDialer)

// OnSuccess sets the onSuccess callback
func OnSuccess(onSuccess func(dialer Dialer)) Option {
	return func(dialer *BanditDialer) {
		dialer.onSuccess = onSuccess
	}
}

// OnError sets the onError callback. When called, it includes an error
// and whether or not bandit has any succeeding dialers
func OnError(onError func(error, bool)) Option {
	return func(dialer *BanditDialer) {
		dialer.onError = onError
	}
}
