package datacap

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/zaplog"
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
	log                = logging.LoggerFor("flashlight.datacap")
)

type dataCap struct {
	iconURL  func() string
	clickURL func() string
	isPro    func() (bool, bool)
}

// ServeDataCap starts serving data cap data to the frontend.
func ServeDataCap(channel ws.UIChannel, iconURL func() string, clickURL func() string, isPro func() (bool, bool)) error {
	helloFn := func(write func(interface{})) {
		log.Infof("Sending current bandwidth quota to new client")
		write(bandwidth.GetQuota())
	}
	bservice, err := channel.Register("bandwidth", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	dc := &dataCap{iconURL: iconURL, clickURL: clickURL, isPro: isPro}
	go func() {
		for quota := range bandwidth.Updates {
			dc.processQuota(bservice.Out, quota)
		}
	}()
	return nil
}

func (dc *dataCap) processQuota(out chan<- interface{}, quota *bandwidth.Quota) {
	log.Infof("Sending update...")
	out <- quota
	isFull := dc.isFull(quota)
	dataCapListenersMx.RLock()
	listeners := dataCapListeners
	dataCapListenersMx.RUnlock()
	for _, l := range listeners {
		l(isFull)
	}
	if isFull {
		oneFull.Do(func() {
			go dc.notifyCapHit()
		})
	} else if dc.isEightyOrMore(quota) {
		oneEighty.Do(func() {
			go dc.notifyEighty()
		})
	} else if dc.isFiftyOrMore(quota) {
		oneFifty.Do(func() {
			go dc.notifyFifty()
		})
	}
}
func AddDataCapListener(l func(hitDataCap bool)) {
	dataCapListenersMx.Lock()
	dataCapListeners = append(dataCapListeners, l)
	dataCapListenersMx.Unlock()
}

func (dc *dataCap) isEightyOrMore(quota *bandwidth.Quota) bool {
	return dc.checkPercent(quota, 0.8)
}

func (dc *dataCap) isFiftyOrMore(quota *bandwidth.Quota) bool {
	return dc.checkPercent(quota, 0.5)
}

func (dc *dataCap) isFull(quota *bandwidth.Quota) bool {
	return (quota.MiBAllowed <= quota.MiBUsed)
}

func (dc *dataCap) checkPercent(quota *bandwidth.Quota, percent float64) bool {
	return (float64(quota.MiBUsed) / float64(quota.MiBAllowed)) > percent
}

func (dc *dataCap) notifyEighty() {
	dc.notifyPercent(80)
}

func (dc *dataCap) notifyFifty() {
	dc.notifyPercent(50)
}

func (dc *dataCap) percentMsg(msg string, percent int) string {
	str := strconv.Itoa(percent) + "%"
	return fmt.Sprintf(msg, str)
}

func (dc *dataCap) notifyPercent(percent int) {
	title := dc.percentMsg(i18n.T("BACKEND_DATA_PERCENT_TITLE"), percent)
	msg := dc.percentMsg(i18n.T("BACKEND_DATA_PERCENT_MESSAGE"), percent)

	dc.notifyFreeUser(title, msg, "data-cap-"+strconv.Itoa(percent))
}

func (dc *dataCap) notifyCapHit() {
	title := i18n.T("BACKEND_DATA_TITLE")
	msg := i18n.T("BACKEND_DATA_MESSAGE")

	dc.notifyFreeUser(title, msg, "data-cap-100")
}

func (dc *dataCap) notifyFreeUser(title, msg, campaign string) {
	if isPro, ok := dc.isPro(); !ok {
		log.Infof("user status is unknown, skip showing notification")
		return
	} else if isPro {
		log.Infof("Not showing desktop notification for pro user")
		return
	}

	note := &notify.Notification{
		Title:    title,
		Message:  msg,
		ClickURL: dc.clickURL(),
		IconURL:  dc.iconURL(),
	}
	_ = notifier.ShowNotification(note, campaign)
}
