package pro

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	url := "https://api.getiantem.org/plans"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		assert.Fail(t, "Could not get request")
	}
	PrepareForFronting(req)

	// Just use the default transport since otherwise test setup is difficult.
	// This means it does not actually touch the proxying code, but that should
	// be tested separately.
	client := getHTTPClient(http.DefaultTransport, http.DefaultTransport)
	res, e := client.Do(req)
	assert.Nil(t, e)

	body, bodyErr := ioutil.ReadAll(res.Body)
	assert.Nil(t, bodyErr)
	assert.True(t, len(body) > 0, "Should have received some body")

	res, e = client.Do(req)
	assert.Nil(t, e)

	body, bodyErr = ioutil.ReadAll(res.Body)
	assert.Nil(t, bodyErr)
	assert.True(t, len(body) > 0, "Should have received some body")
}
