package android

import (
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/pro"
	client "github.com/getlantern/flashlight/pro/client"
	"github.com/stripe/stripe-go"
	"strings"
)

type Session interface {
	config.UserConfig
	SetCountry(string)
	UpdateStats(string, string, string, int, int)
	SetStaging(bool)
	ShowSurvey(string)
	ProxyAll() bool
	BandwidthUpdate(int, int)
	DeviceId() string
	AccountId() string
	AddDevice(string, string)
	AddPlan(string, string, string, bool, int, int)
	Locale() string
	Code() string
	VerifyCode() string
	DeviceCode() string
	DeviceName() string
	Referral() string
	Plan() string
	Provider() string
	ResellerCode() string
	SetSignature(string)
	SetPaymentProvider(string)
	StripeToken() string
	StripeApiKey() string
	Email() string
	SetToken(string)
	SetUserId(int64)
	SetDeviceCode(string, int64)
	UserData(bool, int64, string, string)
	SetCode(string)
	SetError(string, string)
	SetErrorId(string, string)
	Currency() string
	DeviceOS() string
	SetStripePubKey(string)
}

const (
	defaultCurrencyCode = `usd`
)

type proRequest struct {
	client  *client.Client
	user    client.User
	session Session
}

type proFunc func(*proRequest) (*client.Response, error)

func newRequest(session Session) *proRequest {

	httpClient := pro.GetHTTPClient()

	req := &proRequest{
		client: client.NewClient(httpClient),
		user: client.User{
			Auth: client.Auth{
				DeviceID: session.DeviceId(),
				ID:       session.GetUserID(),
				Token:    session.GetToken(),
			},
		},
		session: session,
	}
	req.client.SetLocale(session.Locale())

	return req
}

func newUser(req *proRequest) (*client.Response, error) {

	res, err := req.client.UserCreate(req.user)
	if err != nil {
		log.Errorf("Could not create new Pro user: %v", err)
	} else {
		log.Debugf("Created new user with referral %s user id %v", res.User.Referral, res.User.Auth.ID)
		req.session.SetUserId(res.User.Auth.ID)
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetCode(res.User.Referral)
	}
	return res, err
}

func purchase(req *proRequest) (*client.Response, error) {

	purchase := client.Purchase{
		IdempotencyKey: stripe.NewIdempotencyKey(),
		StripeToken:    req.session.StripeToken(),
		StripeEmail:    req.session.Email(),
		Provider:       req.session.Provider(),
		ResellerCode:   req.session.ResellerCode(),
		Email:          req.session.Email(),
		Plan:           req.session.Plan(),
		Currency:       strings.ToLower(req.session.Currency()),
	}
	pubKey := req.session.StripeApiKey()
	deviceName := req.session.DeviceName()

	return req.client.Purchase(req.user, deviceName, pubKey, purchase)
}

func requestCode(req *proRequest) (*client.Response, error) {

	res, err := req.client.RequestLinkCode(req.user,
		req.session.DeviceName())
	if err != nil {
		log.Errorf("Could not request link code: %v", err)
	} else {
		req.session.SetDeviceCode(res.User.Code, res.User.ExpireAt)
	}
	log.Debugf("Request code response: %v", err)
	return res, err
}

func redeemCode(req *proRequest) (*client.Response, error) {

	req.user.Code = req.session.DeviceCode()
	res, err := req.client.RedeemLinkCode(req.user,
		req.session.DeviceName())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not redeem code: %v", err)
	} else {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func userRecover(req *proRequest) (*client.Response, error) {
	res, err := req.client.UserRecover(req.user,
		req.session.AccountId())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not recover user account: %v", err)
	} else {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func verifyCode(req *proRequest) (*client.Response, error) {
	verifyCode := req.session.VerifyCode()
	log.Debugf("Verify code is %s", verifyCode)
	res, err := req.client.UserLinkValidate(req.user, verifyCode)
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not verify user account: %v", err)
	} else {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func linkRequest(req *proRequest) (*client.Response, error) {
	res, err := req.client.UserLinkRequest(req.user,
		req.session.Email(), req.session.DeviceName())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not send user email: %v", err)
	}
	return res, err
}

func signin(req *proRequest) (*client.Response, error) {

	req.user.Code = req.session.VerifyCode()
	res, err := req.client.ApplyLinkCode(req.user)
	if err != nil {
		log.Errorf("Could not complete signin: %v", err)
	}
	return res, err
}

func referral(req *proRequest) (*client.Response, error) {
	return req.client.RedeemReferralCode(req.user,
		req.session.Referral())
}

func cancel(req *proRequest) (*client.Response, error) {
	return req.client.CancelSubscription(req.user)
}

func plans(req *proRequest) (*client.Response, error) {
	res, err := req.client.Plans(req.user)
	if err != nil || len(res.Plans) == 0 {
		return res, err
	}
	req.session.SetStripePubKey(res.PubKey)
	for _, plan := range res.Plans {
		var currency string
		var price int
		for currency, price = range plan.Price {
			break
		}
		if currency != "" {
			log.Debugf("Calling add plan with %s currency %s desc: %s best value %t price %d",
				plan.Id, currency, plan.Description, plan.BestValue, price)
			req.session.AddPlan(plan.Id, plan.Description,
				currency, plan.BestValue, plan.Duration.Years, price)
		}
	}

	return res, err
}

// Used to confirm an email isn't already associated with a Pro
// account
func emailExists(req *proRequest) (*client.Response, error) {
	res, err := req.client.EmailExists(req.user, req.session.Email())
	if err != nil {
		log.Errorf("Error checking if email exists: %v", err)
	}
	return res, err
}

func userData(req *proRequest) (*client.Response, error) {

	res, err := req.client.UserStatus(req.user)
	if err != nil {
		log.Errorf("Error getting Pro user data: %v", err)
		return res, err
	}
	log.Debugf("User data: %v", res.User)

	deviceLinked := true
	deviceName := req.session.DeviceName()
	deviceId := req.session.DeviceId()

	isActive := res.User.UserStatus == "active"

	if isActive {
		// user is Pro but device may no longer be linked
		deviceLinked = false
	}

	for _, device := range res.User.Devices {
		if device.Name == deviceName || device.Id == deviceId {
			deviceLinked = true
		}
		req.session.AddDevice(device.Id, device.Name)
	}

	req.session.UserData(isActive && deviceLinked,
		res.User.Expiration, res.User.Subscription, res.User.Email)

	return res, err
}

func userUpdate(req *proRequest) (*client.Response, error) {
	res, userId, err := req.client.UserUpdate(req.user,
		req.session.Email())
	if err != nil {
		log.Errorf("Error making user update request: %v", err)
	} else if userId != 0 {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(userId)
	}
	return res, err
}

func pwSignature(req *proRequest) (*client.Response, error) {
	sig, err := req.client.PWSignature(req.user,
		req.session.Email(),
		strings.ToLower(req.session.Currency()),
		req.session.DeviceName(),
		req.session.Plan())

	if err != nil {
		log.Errorf("Error trying to generate pw signature: %v", err)
		return nil, err
	}
	req.session.SetSignature(sig)
	return &client.Response{Status: "ok"}, nil
}

func userPaymentGateway(req *proRequest) (*client.Response, error) {
	provider, err := req.client.UserPaymentGateway(req.user, req.session.DeviceOS())
	if err != nil {
		log.Errorf("Error trying to determine payment provider: %v", err)
		return nil, err
	}
	req.session.SetPaymentProvider(provider)
	return &client.Response{Status: "ok"}, nil
}

func RemoveDevice(deviceId string, session Session) bool {
	req := newRequest(session)
	log.Debugf("Calling user link remove on device %s", deviceId)
	res, err := req.client.UserLinkRemove(req.user, deviceId)
	if err != nil || res.Status != "ok" {
		log.Errorf("Error removing device: %v status: %s", err, res.Status)
		return false
	}

	return true
}

func ProRequest(command string, session Session) bool {

	req := newRequest(session)

	log.Debugf("Received a %s pro request", command)

	commands := map[string]proFunc{
		"emailexists":          emailExists,
		"newuser":              newUser,
		"payment-signature":    pwSignature,
		"user-payment-gateway": userPaymentGateway,
		"purchase":             purchase,
		"plans":                plans,
		"signin":               signin,
		"linkrequest":          linkRequest,
		"redeemcode":           redeemCode,
		"requestcode":          requestCode,
		"userdata":             userData,
		"userrecover":          userRecover,
		"userupdate":           userUpdate,
		"verifycode":           verifyCode,
		"referral":             referral,
		"cancel":               cancel,
	}

	cmd, cmdFound := commands[command]
	if !cmdFound {
		session.SetError(command, "Command not found")
		return false
	}
	res, err := cmd(req)
	if err != nil || res.Status != "ok" {
		log.Errorf("Error making %s request to Pro server: %v response: %v", command, err, res)
		if res != nil {
			session.SetError(command, res.Error)
			session.SetErrorId(command, res.ErrorId)
		}
		return false
	}

	return true
}
