package client

type Purchase struct {
	StripeToken    string `json:"stripeToken"`
	ResellerCode   string `json:"resellerCode"`
	Provider       string `json:"provider"`
	Email          string `json:"email"`
	IdempotencyKey string `json:"idempotencyKey"`
	StripeEmail    string `json:"stripeEmail"`
	Plan           string `json:"plan"`
	Currency       string `json:"currency"`
}
