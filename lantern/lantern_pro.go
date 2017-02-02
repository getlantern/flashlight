package lantern

import (
	"github.com/getlantern/bandwidth"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/pro-server-client/go-client"
	"github.com/stripe/stripe-go"
	"strings"
)

const (
	defaultCurrencyCode = `usd`
)

type Session interface {
	GetUserID() int
	Code() string
	VerifyCode() string
	DeviceCode() string
	DeviceId() string
	DeviceName() string
	Locale() string
	Referral() string
	GetToken() string
	Plan() string
	Provider() string
	ResellerCode() string
	StripeToken() string
	StripeApiKey() string
	Email() string
	AccountId() string
	SetToken(string)
	SetUserId(int)
	SetDeviceCode(string, int64)
	ShowSurvey(string)
	BandwidthUpdate(int, int)
	UserData(bool, int64, string, string)
	SetCode(string)
	SetError(string, string)
	SetErrorId(string, string)
	Currency() string
	SetStripePubKey(string)
	AddPlan(string, string, string, bool, int, int)
	AddDevice(string, string)
}

type proRequest struct {
	proClient *client.Client
	user      client.User
	session   Session
}

type proFunc func(*proRequest) (*client.Response, error)

func newRequest(shouldProxy bool, session Session) (*proRequest, error) {
	httpClient, err := proxied.GetHTTPClient(shouldProxy)
	if err != nil {
		log.Errorf("Could not create HTTP client: %v", err)
		return nil, err
	}

	req := &proRequest{
		proClient: client.NewClient(httpClient),
		user: client.User{
			Auth: client.Auth{
				DeviceID: session.DeviceId(),
				ID:       session.GetUserID(),
				Token:    session.GetToken(),
			},
		},
	}

	return req, nil
}

func newuser(r *proRequest) (*client.Response, error) {
	r.proClient.SetLocale(r.session.Locale())
	res, err := r.proClient.UserCreate(r.user)
	if err != nil {
		log.Errorf("Could not create new Pro user: %v", err)
	} else {
		log.Debugf("Created new user with referral %s user id %v", res.User.Referral, res.User.Auth.ID)
		r.session.SetUserId(res.User.Auth.ID)
		r.session.SetToken(res.User.Auth.Token)
		r.session.SetCode(res.User.Referral)
	}
	return res, err
}

func purchase(r *proRequest) (*client.Response, error) {

	purchase := client.Purchase{
		IdempotencyKey: stripe.NewIdempotencyKey(),
		StripeToken:    r.session.StripeToken(),
		StripeEmail:    r.session.Email(),
		Provider:       r.session.Provider(),
		ResellerCode:   r.session.ResellerCode(),
		Email:          r.session.Email(),
		Plan:           r.session.Plan(),
		Currency:       strings.ToLower(r.session.Currency()),
	}
	pubKey := r.session.StripeApiKey()
	deviceName := r.session.DeviceName()

	return r.proClient.Purchase(r.user, deviceName, pubKey, purchase)
}

func requestcode(r *proRequest) (*client.Response, error) {

	res, err := r.proClient.RequestLinkCode(r.user, r.session.DeviceName())
	if err != nil {
		log.Errorf("Could not request link code: %v", err)
	} else {
		r.session.SetDeviceCode(res.User.Code, res.User.ExpireAt)
	}
	log.Debugf("Request code response: %v", err)
	return res, err
}

func redeemcode(r *proRequest) (*client.Response, error) {
	r.user.Code = r.session.DeviceCode()
	res, err := r.proClient.RedeemLinkCode(r.user, r.session.DeviceName())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not redeem code: %v", err)
	} else {
		r.session.SetToken(res.User.Auth.Token)
		r.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func userrecover(r *proRequest) (*client.Response, error) {
	res, err := r.proClient.UserRecover(r.user, r.session.AccountId())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not recover user account: %v", err)
	} else {
		r.session.SetToken(res.User.Auth.Token)
		r.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func verifycode(r *proRequest) (*client.Response, error) {
	verifyCode := r.session.VerifyCode()
	log.Debugf("Verify code is %s", verifyCode)
	res, err := r.proClient.UserLinkValidate(r.user, verifyCode)
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not verify user account: %v", err)
	} else {
		r.session.SetToken(res.User.Auth.Token)
		r.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func linkrequest(r *proRequest) (*client.Response, error) {
	res, err := r.proClient.UserLinkRequest(r.user, r.session.Email(), r.session.DeviceName())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not send user email: %v", err)
	}
	return res, err
}

func signin(r *proRequest) (*client.Response, error) {
	r.user.Code = r.session.VerifyCode()
	res, err := r.proClient.ApplyLinkCode(r.user)
	if err != nil {
		log.Errorf("Could not complete signin: %v", err)
	}
	return res, err
}

func referral(r *proRequest) (*client.Response, error) {
	return r.proClient.RedeemReferralCode(r.user, r.session.Referral())
}

func cancel(r *proRequest) (*client.Response, error) {
	return r.proClient.CancelSubscription(r.user)
}

func plans(r *proRequest) (*client.Response, error) {
	res, err := r.proClient.Plans(r.user)
	if err != nil || len(res.Plans) == 0 {
		return res, err
	}
	r.session.SetStripePubKey(res.PubKey)
	for _, plan := range res.Plans {
		var currency string
		var price int
		for currency, price = range plan.Price {
			break
		}
		if currency != "" {
			log.Debugf("Calling add plan with %s currency %s desc: %s best value %t price %d",
				plan.Id, currency, plan.Description, plan.BestValue, price)
			r.session.AddPlan(plan.Id, plan.Description, currency, plan.BestValue, plan.Duration.Years, price)
		}
	}

	return res, err
}

func userdata(r *proRequest) (*client.Response, error) {
	res, err := r.proClient.UserData(r.user)
	if err != nil {
		log.Errorf("Error getting Pro user data: %v", err)
		return res, err
	}
	log.Debugf("User data: %v", res.User)

	deviceLinked := true
	deviceName := r.session.DeviceName()
	deviceId := r.session.DeviceId()

	isActive := res.User.UserStatus == "active"

	if isActive {
		// user is Pro but device may no longer be linked
		deviceLinked = false
	}

	for _, device := range res.User.Devices {
		if device.Name == deviceName || device.Id == deviceId {
			deviceLinked = true
		}
		r.session.AddDevice(device.Id, device.Name)
	}

	r.session.UserData(isActive && deviceLinked,
		res.User.Expiration, res.User.Subscription, res.User.Email)

	return res, err
}

func userupdate(r *proRequest) (*client.Response, error) {
	res, userId, err := r.proClient.UserUpdate(r.user, r.session.Email())
	if err != nil {
		log.Errorf("Error making user update request: %v", err)
	} else if userId != 0 {
		r.session.SetToken(res.User.Auth.Token)
		r.session.SetUserId(userId)
	}
	return res, err
}

func RemoveDevice(shouldProxy bool, deviceId string, session Session) bool {
	req, err := newRequest(shouldProxy, session)
	if err != nil {
		log.Errorf("Error creating request: %v", err)
		return false
	}
	log.Debugf("Calling user link remove on device %s", deviceId)
	res, err := req.proClient.UserLinkRemove(req.user, deviceId)
	if err != nil || res.Status != "ok" {
		log.Errorf("Error removing device: %v status: %s", err, res.Status)
		return false
	}

	return true
}

func ProRequest(shouldProxy bool, command string, session Session) bool {

	if command == "survey" {
		url, err := surveyRequest(shouldProxy, session.Locale())
		if err == nil && url != "" {
			session.ShowSurvey(url)
			return true
		} else {
			log.Errorf("Error finding survey: %v", err)
			return false
		}
	} else if command == "bandwidth" {
		percent, remaining := getBandwidth(bandwidth.GetQuota())
		if percent != 0 && remaining != 0 {
			session.BandwidthUpdate(percent, remaining)
		}
		return true
	}

	req, err := newRequest(shouldProxy, session)
	if err != nil {
		log.Errorf("Error creating new request: %v", err)
		return false
	}
	req.session = session

	req.proClient.SetLocale(session.Locale())

	log.Debugf("Received a %s pro request", command)

	commands := map[string]proFunc{
		"newuser":     newuser,
		"purchase":    purchase,
		"plans":       plans,
		"signin":      signin,
		"linkrequest": linkrequest,
		"redeemcode":  redeemcode,
		"requestcode": requestcode,
		"userdata":    userdata,
		"userrecover": userrecover,
		"userupdate":  userupdate,
		"verifycode":  verifycode,
		"referral":    referral,
		"cancel":      cancel,
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
