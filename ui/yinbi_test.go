package ui

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

var s *Server

type PaymentTest struct {
	name             string
	params           PaymentParams
	expectedCode     int
	expectedResponse Response
}

func TestMain(m *testing.M) {
	s = newServer("", common.AuthServerAddr,
		"test-http-token", false)
	s.start(":0")
	code := m.Run()
	os.Exit(code)
}

func TestCreateMnemonic(t *testing.T) {
	url := s.getAPIAddr("/user/mnemonic")
	req, _ := http.NewRequest(GET, url,
		nil)
	resp := httptest.NewRecorder()
	s.createMnemonic(resp, req)
	dumpResponse(resp)
	var r struct {
		Mnemonic string `json:"mnemonic"`
	}
	decodeResp(t, resp, &r)
	words := strings.Split(r.Mnemonic, " ")
	assert.Equal(t, len(words), 24)
}

func decodeResp(t *testing.T,
	resp *httptest.ResponseRecorder,
	r interface{}) {
	body, _ := ioutil.ReadAll(resp.Body)
	err := json.Unmarshal(body, r)
	assert.Nil(t, err)
}

func createPaymentRequest(params PaymentParams) *http.Request {
	requestBody, _ := json.Marshal(params)
	url := s.getAPIAddr("/payment/new")
	req, _ := http.NewRequest(POST, url, bytes.NewBuffer(requestBody))
	req.Header.Add(HeaderContentType, MIMEApplicationJSON)
	return req
}

func testPaymentHandler(t *testing.T, req *http.Request, responseCode int,
	expectedResponse Response) {
	rec := httptest.NewRecorder()
	s.sendPaymentHandler(rec, req)
	dumpResponse(rec)
	var resp Response
	resp.Errors = make(Errors)
	decodeResp(t, rec, &resp)
	assert.Equal(t, rec.Code, http.StatusBadRequest)
	assert.True(t, len(resp.Errors) > 0)
	assert.Equal(t, resp, expectedResponse)
}

func newPaymentParams(username, password, dst,
	amount, asset string) PaymentParams {
	return PaymentParams{
		Username:    username,
		Password:    password,
		Destination: dst,
		Amount:      amount,
		Asset:       asset,
	}
}

func newResponse(err string, errors Errors) Response {
	return Response{
		err,
		errors,
	}
}

func TestSendPayment(t *testing.T) {
	cases := []PaymentTest{
		{
			"MissingUsername",
			newPaymentParams("", "qwejknq2ejnqe", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "13", "YNB"),
			http.StatusBadRequest,
			newResponse("", Errors{
				"username": ErrMissingUsername.Error(),
			}),
		},
		{
			"MissingPassword",
			newPaymentParams("test1234", "", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "13", "YNB"),
			http.StatusBadRequest,
			newResponse("", Errors{
				"password": ErrMissingPassword.Error(),
			}),
		},
		{
			"MissingDestination",
			newPaymentParams("test1234", "qwejknq2ejnqe", "", "13", "YNB"),
			http.StatusBadRequest,
			newResponse("", Errors{
				"destination": ErrMissingDestination.Error(),
			}),
		},
		{
			"Invalid Amount",
			newPaymentParams("test1234", "qwejknq2ejnqe", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "-13", "YNB"),
			http.StatusBadRequest,
			newResponse("", Errors{
				"amount": ErrPaymentInvalidAmount.Error(),
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := createPaymentRequest(tc.params)
			testPaymentHandler(t, req, tc.expectedCode,
				tc.expectedResponse)
		})
	}
}
