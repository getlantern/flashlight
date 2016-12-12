package app

import (
	"time"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/ui"
)

type locationData struct {
	Code string `json:"code"`
}

func serveLocation() {
	helloFn := func(write func(interface{}) error) error {
		// avoid geolookup from blocking the application.
		go func() {
			err := write(locationData{
				Code: geolookup.GetCountry(time.Second * 30),
			})
			if err != nil {
				log.Error(err)
			}
		}()
		return nil
	}

	_, err := ui.Register("location", helloFn)
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
	}
}
