package app

import (
	"strconv"
	"sync"

	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/flashlight/ws"
)

type provider struct {
	PubKey string `json:"pubKey"`
}

type purchase struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Plan   string `json:"plan"`
}

type device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

type user struct {
	UserID      int64      `json:"userId"`
	Email       string     `json:"email"`
	UserStatus  string     `json:"userStatus"`
	Locale      string     `json:"locale"`
	Expiration  int64      `json:"expiration"`
	BonusMonths int64      `json:"bonusMonths"`
	Invitees    []int64    `json:"invitees"`
	Inviters    []int64    `json:"inviters"`
	Servers     []string   `json:"servers"`
	Purchases   []purchase `json:"purchases"`
	Referral    string     `json:"code"`
	Devices     []device   `json:"devices"`
}

//session implements the Session interface of package flashlight/pro.
//It is also unmarshalled to json and send to desktop UI.
type session struct {
	GotUserData bool `json:"gotUserData"`
	User        user `json:"user"`

	// Providers                map[string]provider `json:"providers"`
	StripePubKey             string        `json:"stripePubKey"`
	CurrentProviderSignature string        `json:"signature"`
	Plans                    []client.Plan `json:"plans"`
	IsPro                    bool          `json:"isPro"`

	CmdErrors   map[string]string `json:"cmdErrors"`
	CmdErrorIDs map[string]string `json:"cmdErrorIds"`

	cmdParams map[string]interface{} `json:"-"`
	signal    chan struct{}          `json:"-"`
	mu        sync.RWMutex           `json:"-"`
}

var theSession = &session{
	CmdErrors:   make(map[string]string),
	CmdErrorIDs: make(map[string]string),
	signal:      make(chan struct{}),
}

func (s *session) GetUserID() int64 {
	return settings.GetUserID()
}

func (s *session) GetToken() string {
	return settings.GetToken()
}

func (s *session) DeviceId() string {
	return settings.GetDeviceID()
}

func (s *session) AccountId() string {
	return strconv.FormatInt(s.GetUserID(), 10)
}

func (s *session) AddDevice(id string, name string) {
	s.mu.Lock()
	s.User.Devices = append(s.User.Devices, device{ID: id, Name: name})
	s.mu.Unlock()
	s.notify()
}

func (s *session) AddPlan(id string, desc string, currency string,
	bestValue bool, years int, price int) {
	s.mu.Lock()
	s.Plans = append(s.Plans, client.Plan{
		Id:          id,
		Description: desc,
		Price:       map[string]int{currency: price},
		BestValue:   bestValue,
		Duration:    client.Duration{Years: years}, // TODO: calc correct duration
	})
	s.mu.Unlock()
	s.notify()
}

func (s *session) Locale() string {
	return ""
}

func (s *session) Code() string {
	return ""
}

func (s *session) VerifyCode() string {
	return ""
}

func (s *session) DeviceCode() string {
	return ""
}

func (s *session) DeviceOS() string {
	return ""
}

func (s *session) DeviceName() string {
	return ""
}

func (s *session) Referral() string {
	return ""
}

func (s *session) Plan() string {
	return ""
}

func (s *session) Provider() string {
	return ""
}

func (s *session) ResellerCode() string {
	return ""
}

func (s *session) SetPaymentProvider(string) {
	s.notify()
}

func (s *session) SetSignature(string) {
	s.notify()
}

func (s *session) StripeToken() string {
	return ""
}

func (s *session) StripeApiKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.StripePubKey
}

func (s *session) Email() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, exists := s.cmdParams["email"]; exists {
		if email, ok := v.(string); ok {
			return email
		}
	}
	return ""
}

func (s *session) SetToken(token string) {
	settings.setString(SNUserToken, token)
	s.notify()
}

func (s *session) SetUserId(id int64) {
	settings.SetUserID(id)
	s.mu.Lock()
	s.User.UserID = id
	s.mu.Unlock()
	s.notify()
}

func (s *session) SetDeviceCode(string, int64) {
	s.notify()
}

func (s *session) UserData(isPro bool, expiration int64, subscription string, email string) {
	s.mu.Lock()
	s.GotUserData = true
	s.IsPro = isPro
	s.User.Expiration = expiration
	// s.User.Subscription = subscription
	s.User.Email = email
	s.mu.Unlock()
}

func (s *session) SetCode(string) {
	s.notify()
}

func (s *session) SetError(cmd string, err string) {
	s.mu.Lock()
	s.CmdErrors[cmd] = err
	s.mu.Unlock()
	s.notify()
}

func (s *session) SetErrorId(cmd string, id string) {
	s.mu.Lock()
	s.CmdErrorIDs[cmd] = id
	s.mu.Unlock()
	s.notify()
}

func (s *session) Currency() string {
	return ""
}

func (s *session) SetStripePubKey(key string) {
	s.mu.Lock()
	s.StripePubKey = key
	s.mu.Unlock()
	s.notify()
}

func (s *session) notify() {
	s.signal <- struct{}{}
}

func (s *session) clearError(cmd string) {
	s.mu.Lock()
	s.CmdErrors[cmd] = ""
	s.CmdErrorIDs[cmd] = ""
	s.mu.Unlock()
}

func (s *session) setParams(params map[string]interface{}) {
	s.mu.Lock()
	s.cmdParams = params
	s.mu.Unlock()
}

func (s *session) copy() (ret session) {
	s.mu.RLock()
	ret = *s
	ret.CmdErrors = make(map[string]string)
	for k, v := range s.CmdErrors {
		ret.CmdErrors[k] = v
	}
	ret.CmdErrorIDs = make(map[string]string)
	for k, v := range s.CmdErrorIDs {
		ret.CmdErrorIDs[k] = v
	}
	s.mu.RUnlock()
	return
}

func servePro() error {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending current user data to new client: %v", theSession)
		write(theSession)
	}
	service, err := ws.Register("pro", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	go func() {
		for _ = range theSession.signal {
			service.Out <- theSession.copy()
		}
	}()
	go func() {
		for m := range service.In {
			message, ok := m.(map[string]interface{})
			if !ok {
				log.Errorf("Unrecognized pro message %v", m)
				continue
			}
			cmd, ok := message["cmd"].(string)
			if !ok {
				log.Errorf("Unrecognized pro command %v", message["cmd"])
				continue
			}
			theSession.clearError(cmd)
			params, _ := message["params"].(map[string]interface{})
			if params != nil {
				log.Debugf("Setting pro parameters to %v", params)
				theSession.setParams(params)
			}
			pro.ProRequest(cmd, theSession)
		}
	}()
	// pro package doesn't call SetUserID() by default
	theSession.User.UserID = theSession.GetUserID()
	pro.InitSession(theSession)

	return nil
}
