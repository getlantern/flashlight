package app

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/notifier"

	"github.com/getlantern/flashlight/notifier"
	"github.com/getlantern/flashlight/ws"
)

var (
	// These just make sure we only sent a single notification at each percentage
	// level.
	oneFifty  = &sync.Once{}
	oneEighty = &sync.Once{}
	oneFull   = &sync.Once{}

	dataCapListeners   = make([]func(hitDataCap bool), 0)
	dataCapListenersMx sync.RWMutex
	logger             = golog.LoggerFor("flashlight.app.bandwidth")
)

type notifyStatus struct {
	iconURL  func() string
	clickURL func() string
}

func addDataCapListener(l func(hitDataCap bool)) {
	dataCapListenersMx.Lock()
	dataCapListeners = append(dataCapListeners, l)
	dataCapListenersMx.Unlock()
}

func serveBandwidth(iconURL func() string, clickURL func() string) error {
	helloFn := func(write func(interface{})) {
		logger.Debugf("Sending current bandwidth quota to new client")
		write(bandwidth.GetQuota())
	}
	bservice, err := ws.Register("bandwidth", helloFn)
	if err != nil {
		logger.Errorf("Error registering with UI? %v", err)
		return err
	}
	ns := notifyStatus{iconURL, clickURL}
	go func() {
		for quota := range bandwidth.Updates {
			logger.Debugf("Sending update...")
			bservice.Out <- quota
			isFull := ns.isFull(quota)
			dataCapListenersMx.RLock()
			listeners := dataCapListeners
			dataCapListenersMx.RUnlock()
			for _, l := range listeners {
				l(isFull)
			}
			if isFull {
				oneFull.Do(func() {
					go ns.notifyCapHit()
				})
			} else if ns.isEightyOrMore(quota) {
				oneEighty.Do(func() {
					go ns.notifyEighty()
				})
			} else if ns.isFiftyOrMore(quota) {
				oneFifty.Do(func() {
					go ns.notifyFifty()
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

func (s *notifyStatus) notifyEighty() {
	s.notifyPercent(80)
}

func (s *notifyStatus) notifyFifty() {
	s.notifyPercent(50)
}

func (s *notifyStatus) percentMsg(msg string, percent int) string {
	str := strconv.Itoa(percent) + "%"
	return fmt.Sprintf(msg, str)
}

func (s *notifyStatus) notifyPercent(percent int) {
	title := s.percentMsg(i18n.T("BACKEND_DATA_PERCENT_TITLE"), percent)
	msg := s.percentMsg(i18n.T("BACKEND_DATA_PERCENT_MESSAGE"), percent)

	s.notifyFreeUser(title, msg, "data-cap-"+strconv.Itoa(percent))
}

func (s *notifyStatus) notifyCapHit() {
	title := i18n.T("BACKEND_DATA_TITLE")
	msg := i18n.T("BACKEND_DATA_MESSAGE")

	s.notifyFreeUser(title, msg, "data-cap-100")
}

func (s *notifyStatus) notifyFreeUser(title, msg, campaign string) {
	if isPro, ok := isProUser(); !ok {
		logger.Debugf("user status is unknown, skip showing notification")
		return
	} else if isPro {
		logger.Debugf("Not showing desktop notification for pro user")
		return
	}

	note := &notify.Notification{
		Title:    title,
		Message:  msg,
		ClickURL: s.clickURL(),
		IconURL:  s.iconURL(),
	}
	_ = notifier.ShowNotification(note, campaign)
}
