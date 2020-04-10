package yinbi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/ui/handlers"
	"github.com/getlantern/flashlight/ui/params"
	"github.com/getlantern/flashlight/ui/testutils"
	"github.com/getlantern/lantern-server/common"
	"github.com/stretchr/testify/assert"
)

type PaymentTest struct {
	name             string
	params           PaymentParams
	expectedCode     int
	expectedResponse params.Response
}

func newYinbiHandler(t *testing.T) YinbiHandler {
	testutils.StartTestServer(t, common.AuthServerAddr, ":0")
	return New(handlers.Params{
		AuthServerAddr: common.AuthServerAddr,
		HttpClient:     &http.Client{},
	})
}

func TestCreateMnemonic(t *testing.T) {
	h := newYinbiHandler(t)
	url := h.GetAuthAddr("/user/mnemonic")
	req, _ := http.NewRequest(common.GET, url,
		nil)
	resp := httptest.NewRecorder()
	h.createMnemonic(resp, req)
	testutils.DumpResponse(resp)
	var r struct {
		Mnemonic string `json:"mnemonic"`
	}
	testutils.DecodeResp(t, resp, &r)
	words := strings.Split(r.Mnemonic, " ")
	assert.Equal(t, len(words), 24)
}

func createPaymentRequest(h YinbiHandler, params PaymentParams) *http.Request {
	requestBody, _ := json.Marshal(params)
	url := h.GetAuthAddr("/payment/new")
	req, _ := http.NewRequest(common.POST, url, bytes.NewBuffer(requestBody))
	req.Header.Add(common.HeaderContentType, common.MIMEApplicationJSON)
	return req
}

func testPaymentHandler(t *testing.T,
	h YinbiHandler, req *http.Request,
	hasErrors bool, pt PaymentTest) {
	rec := httptest.NewRecorder()
	h.sendPaymentHandler(rec, req)
	testutils.DumpResponse(rec)
	var resp params.Response
	resp.Errors = make(params.Errors)
	testutils.DecodeResp(t, rec, &resp)
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

func TestSendPayment(t *testing.T) {
	h := newYinbiHandler(t)
	cases := []PaymentTest{
		{
			"MissingUsername",
			newPaymentParams("", "qwejknq2ejnqe", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "13", "YNB"),
			http.StatusBadRequest,
			params.NewResponse("", params.Errors{
				"username": ErrMissingUsername.Error(),
			}),
		},
		{
			"MissingPassword",
			newPaymentParams("test1234", "", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "13", "YNB"),
			http.StatusBadRequest,
			params.NewResponse("", params.Errors{
				"password": ErrMissingPassword.Error(),
			}),
		},
		{
			"MissingDestination",
			newPaymentParams("test1234", "qwejknq2ejnqe", "", "13", "YNB"),
			http.StatusBadRequest,
			params.NewResponse("", params.Errors{
				"destination": ErrMissingDestination.Error(),
			}),
		},
		{
			"Invalid Amount",
			newPaymentParams("test1234", "qwejknq2ejnqe", "GBTAXA2E6QE3WHOZGSRNTFC64VGUL4FJQ2FP3OFCROTPWHE5RS6IBI6W", "-13", "YNB"),
			http.StatusBadRequest,
			params.NewResponse("", params.Errors{
				"amount": ErrPaymentInvalidAmount.Error(),
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := createPaymentRequest(h, tc.params)
			testPaymentHandler(t, h, req, true, tc)
		})
	}
}
