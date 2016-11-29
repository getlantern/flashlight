package app

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/i18n"
	"github.com/getlantern/notifier"
	proClient "github.com/getlantern/pro-server-client/go-client"

	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/ui"
)

var (
	// These just make sure we only sent a single notification at each percentage
	// level.
	oneFifty  = &sync.Once{}
	oneEighty = &sync.Once{}
	oneFull   = &sync.Once{}
	ns        = notifyStatus{}
)

type notifyStatus struct {
}

func serveBandwidth() error {
	helloFn := func(write func(interface{}) error) error {
		log.Debugf("Sending current bandwidth quota to new client")
		return write(bandwidth.GetQuota())
	}
	service, err := ui.Register("bandwidth", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	go func() {
		n := notify.NewNotifications()
		for quota := range bandwidth.Updates {
			log.Debugf("Sending update...")
			service.Out <- quota
			if ns.isFull(quota) {
				oneFull.Do(func() {
					go ns.notifyCapHit(n)
				})
			} else if ns.isEightyOrMore(quota) {
				oneEighty.Do(func() {
					go ns.notifyEighty(n)
				})
			} else if ns.isFiftyOrMore(quota) {
				oneFifty.Do(func() {
					go ns.notifyFifty(n)
				})
			}
		}
	}()
	return nil
}

func (s *notifyStatus) isEightyOrMore(quota *bandwidth.Quota) bool {
	return s.checkPercent(quota, 0.8)
}

func (s *notifyStatus) isFiftyOrMore(quota *bandwidth.Quota) bool {
	return s.checkPercent(quota, 0.5)
}

func (s *notifyStatus) isFull(quota *bandwidth.Quota) bool {
	return (quota.MiBAllowed <= quota.MiBUsed)
}

func (s *notifyStatus) checkPercent(quota *bandwidth.Quota, percent float64) bool {
	return (float64(quota.MiBUsed) / float64(quota.MiBAllowed)) > percent
}

func (s *notifyStatus) notifyEighty(n notify.Notifier) {
	s.notifyPercent(80, n)
}

func (s *notifyStatus) notifyFifty(n notify.Notifier) {
	s.notifyPercent(50, n)
}

func (s *notifyStatus) percentMsg(msg string, percent int) string {
	str := strconv.Itoa(percent) + "%"
	return fmt.Sprintf(msg, str)
}

func (s *notifyStatus) notifyPercent(percent int, n notify.Notifier) {
	title := s.percentMsg(i18n.T("BACKEND_DATA_PERCENT_TITLE"), percent)
	msg := s.percentMsg(i18n.T("BACKEND_DATA_PERCENT_MESSAGE"), percent)

	s.notifyFreeUser(n, title, msg)
}

func (s *notifyStatus) notifyCapHit(n notify.Notifier) {
	title := i18n.T("BACKEND_DATA_TITLE")
	msg := i18n.T("BACKEND_DATA_MESSAGE")

	s.notifyFreeUser(n, title, msg)
}

func (s *notifyStatus) notifyFreeUser(n notify.Notifier, title, msg string) {
	userID := settings.GetUserID()
	status, err := s.userStatus(settings.GetDeviceID(), int(userID), settings.GetToken())
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return
	}
	log.Debugf("User %d is %v", userID, status)
	if status == "active" {
		log.Debugf("Not showing desktop notification for pro user")
		return
	}

	logo := ui.AddToken("/img/lantern_logo.png")
	note := &notify.Notification{
		Title:    title,
		Message:  msg,
		ClickURL: ui.GetPreferredUIAddr(),
		IconURL:  logo,
	}

	if err = n.Notify(note); err != nil {
		log.Errorf("Could not notify? %v", err)
		return
	}
}

func (s *notifyStatus) userStatus(deviceID string, userID int, proToken string) (string, error) {
	log.Debugf("Fetching user status with user ID '%v' and pro token '%v'", userID, proToken)
	user := proClient.User{Auth: proClient.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	http, err := proxied.GetHTTPClient(true)
	if err != nil {
		log.Errorf("Unable to get proxied HTTP client: %v", err)
		return "", err
	}
	client := proClient.NewClient(http)
	resp, err := client.UserData(user)
	if err != nil {
		log.Errorf("Fail to get user data: %v", err)
		return "", err
	}
	if resp.Status == "error" {
		return "", errors.New(resp.Error)
	}
	return resp.User.UserStatus, nil
}
