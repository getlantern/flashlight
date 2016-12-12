package app

import (
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/ui"
	"time"
)

type locationData struct {
	Code string `json:"code"`
}

func serveLocation() error {
	helloFn := func(write func(interface{}) error) error {
		return write(locationData{
			Code: geolookup.GetCountry(time.Second * 30),
		})
	}

	_, err := ui.Register("location", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	return nil
}
