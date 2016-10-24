package app

import (
	"github.com/keighl/mandrill"

	"github.com/getlantern/flashlight/ui"
)

type issueReporter struct {
}

var (
	reporter issueReporter
)

func serveIssueReporter() error {
	helloFn := func(write func(interface{}) error) error {
		return nil
	}

	if service, err := ui.Register("issue", helloFn); err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	} else {
		return nil
	}
	go reporter.read(service)
	return nil
}

func (s *issueReporter) read(service *ui.Service) {
	for message := range service.In {
		data, ok := (message).(map[string]interface{})
		if !ok {
			log.Errorf("Malformatted message %v", message)
			continue
		}
		s.send(data)

		service.Out <- message
	}
}

func (s *issueReporter) send(map[string]interface{}) {
	client := mandrill.ClientWithKey("XXXXXXXXX")
	msg := &mandrill.Message{}
	msg.GlobalMergeVars = mandrill.MapToVars(map[string]interface{}{"name": "Bob"})
	client.MessagesSendTemplate()
}
