package ui

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlantern/lantern-server/common"
	"github.com/stretchr/testify/assert"
)

type PaymentTest struct {
	name             string
	params           PaymentParams
	expectedCode     int
	expectedResponse Response
}

func TestCreateMnemonic(t *testing.T) {
	s := startServer(t, common.AuthServerAddr, ":0")
	url := s.getAPIAddr("/user/mnemonic")
	req, _ := http.NewRequest(common.GET, url,
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

func createPaymentRequest(s *Server, params PaymentParams) *http.Request {
	requestBody, _ := json.Marshal(params)
	url := s.getAPIAddr("/payment/new")
	req, _ := http.NewRequest(common.POST, url, bytes.NewBuffer(requestBody))
	req.Header.Add(common.HeaderContentType, common.MIMEApplicationJSON)
	return req
}

func testPaymentHandler(t *testing.T,
	s *Server, req *http.Request,
	hasErrors bool, pt PaymentTest) {
	rec := httptest.NewRecorder()
	s.sendPaymentHandler(rec, req)
	dumpResponse(rec)
	var resp Response
	resp.Errors = make(Errors)
	decodeResp(t, rec, &resp)
	assert.Equal(t, rec.Code, pt.expectedCode)
	assert.Equal(t, len(resp.Errors) > 0, hasErrors)
	assert.Equal(t, resp, pt.expectedResponse)
}

func newPaymentParams(username, password, dst,
	amount, asset string) PaymentParams {
	return PaymentParams{
		AuthParams: AuthParams{
			Username: username,
			Password: password,
		},
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
	s := startServer(t, common.AuthServerAddr, ":0")
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
			req := createPaymentRequest(s, tc.params)
			testPaymentHandler(t, s, req, true, tc)
		})
	}
}
