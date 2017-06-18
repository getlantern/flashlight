package android

import (
	"github.com/getlantern/bandwidth"
	client "github.com/getlantern/flashlight/pro/client"
	"github.com/stripe/stripe-go"
	"strings"
	"time"
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
	UserData(bool, int64, int64, string, string)
	SetCode(string)
	SetError(string, string)
	SetErrorId(string, string)
	Currency() string
	SetStripePubKey(string)
	AddPlan(string, string, string, bool, int, int)
	AddDevice(string, string)
}

const (
	defaultCurrencyCode = `usd`
)

type proRequest struct {
	*client.ProRequest
	session Session
}

type proFunc func(*proRequest) (*client.Response, error)

func newUser(req *proRequest) (*client.Response, error) {

	res, err := req.Client.UserCreate(req.User)
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

	return req.Client.Purchase(req.User, deviceName, pubKey, purchase)
}

func requestCode(req *proRequest) (*client.Response, error) {

	res, err := req.Client.RequestLinkCode(req.User,
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

	req.User.Code = req.session.DeviceCode()
	res, err := req.Client.RedeemLinkCode(req.User,
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
	res, err := req.Client.UserRecover(req.User,
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
	res, err := req.Client.UserLinkValidate(req.User, verifyCode)
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not verify user account: %v", err)
	} else {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(res.User.Auth.ID)
	}
	return res, err
}

func linkRequest(req *proRequest) (*client.Response, error) {
	res, err := req.Client.UserLinkRequest(req.User,
		req.session.Email(), req.session.DeviceName())
	if err != nil || res.Status != "ok" {
		log.Errorf("Could not send user email: %v", err)
	}
	return res, err
}

func signin(req *proRequest) (*client.Response, error) {

	req.User.Code = req.session.VerifyCode()
	res, err := req.Client.ApplyLinkCode(req.User)
	if err != nil {
		log.Errorf("Could not complete signin: %v", err)
	}
	return res, err
}

func referral(req *proRequest) (*client.Response, error) {
	return req.Client.RedeemReferralCode(req.User,
		req.session.Referral())
}

func cancel(req *proRequest) (*client.Response, error) {
	return req.Client.CancelSubscription(req.User)
}

func plans(req *proRequest) (*client.Response, error) {
	res, err := req.Client.Plans(req.User)
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
	res, err := req.Client.EmailExists(req.User, req.session.Email())
	if err != nil {
		log.Errorf("Error checking if email exists: %v", err)
	}
	return res, err
}

func userData(req *proRequest) (*client.Response, error) {

	res, err := client.UserStatus(req.ProRequest)
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

	expiry := time.Unix(res.User.Expiration, 0)
	dur := expiry.Sub(time.Now())
	years := dur.Hours() / 24 / 365
	monthsLeft := int64(years * 12)

	req.session.UserData(isActive && deviceLinked,
		res.User.Expiration, monthsLeft,
		res.User.Subscription, res.User.Email)

	return res, err
}

func userUpdate(req *proRequest) (*client.Response, error) {
	res, userId, err := req.Client.UserUpdate(req.User,
		req.session.Email())
	if err != nil {
		log.Errorf("Error making user update request: %v", err)
	} else if userId != 0 {
		req.session.SetToken(res.User.Auth.Token)
		req.session.SetUserId(userId)
	}
	return res, err
}

func RemoveDevice(deviceId string, session Session) bool {
	user := client.User{
		Auth: client.Auth{
			DeviceID: session.DeviceId(),
			ID:       session.GetUserID(),
			Token:    session.GetToken(),
		},
	}

	req, err := client.NewRequest(user)
	if err != nil {
		log.Errorf("Error creating request: %v", err)
		return false
	}
	log.Debugf("Calling user link remove on device %s", deviceId)
	res, err := req.Client.UserLinkRemove(req.User, deviceId)
	if err != nil || res.Status != "ok" {
		log.Errorf("Error removing device: %v status: %s", err, res.Status)
		return false
	}

	return true
}

func ProRequest(command string, session Session) bool {

	if command == "survey" {
		url, err := surveyRequest(session.Locale())
		if err == nil && url != "" {
			session.ShowSurvey(url)
			return true
		}
		return false
	} else if command == "bandwidth" {
		percent, remaining := getBandwidth(bandwidth.GetQuota())
		if percent != 0 && remaining != 0 {
			session.BandwidthUpdate(percent, remaining)
		}
		return true
	}

	user := client.User{
		Auth: client.Auth{
			DeviceID: session.DeviceId(),
			ID:       session.GetUserID(),
			Token:    session.GetToken(),
		},
	}

	req, err := client.NewRequest(user)
	if err != nil {
		log.Errorf("Error creating new request: %v", err)
		return false
	}
	req.Client.SetLocale(session.Locale())

	log.Debugf("Received a %s pro request", command)

	commands := map[string]proFunc{
		"emailexists": emailExists,
		"newuser":     newUser,
		"purchase":    purchase,
		"plans":       plans,
		"signin":      signin,
		"linkrequest": linkRequest,
		"redeemcode":  redeemCode,
		"requestcode": requestCode,
		"userdata":    userData,
		"userrecover": userRecover,
		"userupdate":  userUpdate,
		"verifycode":  verifyCode,
		"referral":    referral,
		"cancel":      cancel,
	}

	cmd, cmdFound := commands[command]
	if !cmdFound {
		session.SetError(command, "Command not found")
		return false
	}
	res, err := cmd(&proRequest{req, session})
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
