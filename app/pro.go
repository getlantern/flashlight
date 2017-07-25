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

	signal chan struct{} `json:"-"`
	mu     sync.RWMutex  `json:"-"`
}

var theSession = &session{
	signal: make(chan struct{}),
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

func (s *session) StripeApiKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.StripePubKey
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

func (s *session) SetStripePubKey(key string) {
	s.mu.Lock()
	s.StripePubKey = key
	s.mu.Unlock()
	s.notify()
}

func (s *session) notify() {
	s.signal <- struct{}{}
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

	if session.GetUserID() == 0 {
		// create new user first if we have no valid user id
		_, err := newUser(newRequest(session))
		if err != nil {
			log.Errorf("Could not create new pro user")
			return
		}
	}
	req := newRequest(session)

	for _, proFn := range []proFunc{plans, userData} {
		_, err := proFn(req)
		if err != nil {
			log.Errorf("Error making pro request: %v", err)
		}
	}

	log.Debugf("New Lantern session with user id %d", session.GetUserID())

	// pro package doesn't call SetUserID() by default
	theSession.User.UserID = theSession.GetUserID()
	pro.InitSession(theSession)

	return nil
}
