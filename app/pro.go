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
	User                     user                `json:"user"`
	Providers                map[string]provider `json:"providers"`
	CurrentProviderSignature string              `json:"signature"`
	Plans                    []client.Plan       `json:"plans"`
	IsPro                    bool                `json:"isPro"`
	CmdErrors                map[string]string   `json:"cmdErrors"`
	CmdErrorIDs              map[string]string   `json:"cmdErrorIds"`

	signal chan struct{} `json:"-"`
	mu     sync.RWMutex  `json:"-"`
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

func (s *session) SetSignature(string) {
	s.notify()
}

func (s *session) StripeToken() string {
	return ""
}

func (s *session) StripeApiKey() string {
	return ""
}

func (s *session) Email() string {
	return s.User.Email
}

func (s *session) SetToken(token string) {
	settings.setString(SNUserToken, token)
	s.notify()
}

func (s *session) SetUserId(id int64) {
	settings.SetUserID(id)
	s.notify()
}

func (s *session) SetDeviceCode(string, int64) {
	s.notify()
}

func (s *session) UserData(isPro bool, expiration int64, subscription string, email string) {
	s.IsPro = isPro
	s.User.Expiration = expiration
	// s.User.Subscription = subscription
	s.User.Email = email
}

func (s *session) SetCode(string) {
	s.notify()
}

func (s *session) SetError(cmd string, err string) {
	s.CmdErrors[cmd] = err
	s.notify()
}

func (s *session) SetErrorId(cmd string, id string) {
	s.CmdErrorIDs[cmd] = id
	s.notify()
}

func (s *session) Currency() string {
	return ""
}

func (s *session) SetStripePubKey(string) {
	s.notify()
}

func (s *session) notify() {
	s.signal <- struct{}{}
}

func (s *session) copy() (ret session) {
	s.mu.RLock()
	ret = *s
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
		for cmd := range service.In {
			pro.ProRequest(cmd.(string), theSession)
		}
	}()
	pro.InitSession(theSession)

	return nil
}
