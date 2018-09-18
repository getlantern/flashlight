package pro

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/getlantern/zaplog"
	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	log := zaplog.LoggerFor("pro-http-test")
	url := "https://api.getiantem.org/plans"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		assert.Fail(t, "Could not get request")
	}

	// Just use the default transport since otherwise test setup is difficult.
	// This means it does not actually touch the proxying code, but that should
	// be tested separately.
	client := getHTTPClient(http.DefaultTransport, http.DefaultTransport)
	res, e := client.Do(req)

	if !assert.NoError(t, e) {
		return
	}
	log.Infof("Got responsde code: %v", res.StatusCode)
	assert.NotNil(t, res.Body, "nil plans response body?")

	body, bodyErr := ioutil.ReadAll(res.Body)
	assert.Nil(t, bodyErr)
	assert.True(t, len(body) > 0, "Should have received some body")

	res, e = client.Do(req)
	assert.Nil(t, e)

	body, bodyErr = ioutil.ReadAll(res.Body)
	assert.Nil(t, bodyErr)
	assert.True(t, len(body) > 0, "Should have received some body")
}
