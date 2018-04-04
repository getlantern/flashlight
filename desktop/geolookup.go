package desktop

import (
	"time"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/ws"
)

type locationData struct {
	Code string `json:"code"`
}

func serveLocation(channel ws.UIChannel) {
	helloFn := func(write func(interface{})) {
		write(locationData{
			Code: geolookup.GetCountry(time.Second * 30),
		})
	}

	_, err := channel.Register("location", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
	}
}
