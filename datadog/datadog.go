package datadog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

type contextKey string

var (
	apiClient *datadog.APIClient
	mu        sync.Mutex

	clientKey = contextKey("client")

	log = golog.LoggerFor("lantern-desktop.datadog")
)

func Init() error {
	mu.Lock()
	defer mu.Unlock()
	configuration := datadog.NewConfiguration()
	// Configure proxy
	configuration.HTTPClient = &http.Client{
		Transport: proxied.ChainedThenFronted(),
	}
	apiClient = datadog.NewAPIClient(configuration)
	return nil
}

// newErrorEvent creates a new error event to send to Dataodg
func newErrorEvent(err error) *datadogV1.Event {
	text := err.Error()
	alertType := datadogV1.EVENTALERTTYPE_ERROR
	return &datadogV1.Event{
		Title:     datadog.PtrString("Error occurred"),
		Text:      &text,
		AlertType: &alertType,
		Tags:      []string{"lantern-desktop", "error"},
	}
}

// ClientFromContext returns client and indication if it was successful.
func ClientFromContext(ctx context.Context) (*datadog.APIClient, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(clientKey)
	if c, ok := v.(*datadog.APIClient); ok {
		return c, true
	}
	return nil, false
}

// Client returns client from context.
func Client(ctx context.Context) *datadog.APIClient {
	c, ok := ClientFromContext(ctx)
	if !ok {
		log.Fatal("client is not configured")
	}
	return c
}

func sendRequest(ctx context.Context, method, url string, payload []byte) (*http.Response, []byte, error) {
	baseURL := ""
	if !strings.HasPrefix(url, "https://") {
		var err error
		baseURL, err = Client(ctx).GetConfig().ServerURLWithContext(ctx, "")
		if err != nil {
			return nil, []byte{}, fmt.Errorf("failed to get base URL for Datadog API: %s", err.Error())
		}
	}

	request, err := http.NewRequest(method, baseURL+url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, []byte{}, fmt.Errorf("failed to create request for Datadog API: %s", err.Error())
	}
	keys := ctx.Value(datadog.ContextAPIKeys).(map[string]datadog.APIKey)
	request.Header.Add("DD-API-KEY", keys["apiKeyAuth"].Key)
	request.Header.Add("DD-APPLICATION-KEY", keys["appKeyAuth"].Key)
	request.Header.Set("Content-Type", "application/json")

	resp, respErr := Client(ctx).GetConfig().HTTPClient.Do(request)
	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		respErr = fmt.Errorf("failed reading response body: %s", rerr.Error())
	}
	return resp, body, respErr
}

// SendErrorEvent sends a new error event to Dataodg
func SendErrorEvent(err error) {
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: os.Getenv("DD_CLIENT_API_KEY"),
			},
			"appKeyAuth": {
				Key: os.Getenv("DD_CLIENT_APP_KEY"),
			},
		},
	)
	// Send the event to Datadog
	marshalledEvent, _ := json.Marshal(newErrorEvent(err))
	if _, _, err := sendRequest(ctx, "POST", "/api/v1/events", marshalledEvent); err != nil {
		log.Errorf("Failed to send error event to Datadog:", err)
	}
}
