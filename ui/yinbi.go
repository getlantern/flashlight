package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/yinbi-server/config"
	"github.com/getlantern/yinbi-server/crypto"
	"github.com/getlantern/yinbi-server/params"
	"github.com/getlantern/yinbi-server/yinbi"
	"github.com/stellar/go/keypair"
)

const (
	issuer = "GBR6Z4EEHLWASJY3A26IY5AE2K7B6K2F2GDZ2DW44ZWTBTEZYD4KPPA2"
)

var (
	ErrInvalidMnemonic = errors.New("The provided words do not comprise a valid mnemonic")
)

func newYinbiClient(httpClient *http.Client) *yinbi.Client {
	return yinbi.New(params.Params{
		HttpClient: httpClient,
		Config: &config.Config{
			NetworkName: "test",
			AssetCode:   "YNB",
		},
	})
}

func (s *Server) createMnemonic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mnemonic": crypto.NewMnemonic(),
		"success":  true,
	})
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (s *Server) sendPaymentHandler(w http.ResponseWriter,
	req *http.Request) {
	var params PaymentParams
	err := decodeJSONRequest(req, &params)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	errors := params.Validate()
	if len(errors) > 0 {
		s.errorHandler(w, errors, http.StatusBadRequest)
		return
	}
	// get key from keystore here
	secretKey, err := s.keystore.GetKey(params.Username, params.Password)
	if err != nil {
		err = fmt.Errorf("Error retrieving secret key: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	// create a new keypair from the secret key
	// once we have the keypair, we can sign transactions
	pair, err := keypair.Parse(secretKey)
	if err != nil {
		err = fmt.Errorf("Error parsing keypair: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrieved keypair for %s", params.Username)
	resp, err := s.yinbiClient.SendPayment(
		params.Destination,
		params.Amount,
		params.Asset,
		pair.(*keypair.Full),
	)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"tx_id":   resp.Hash,
	})
	return
}

// getAccountDetails is the handler used to look up
// the balances and transaction history for a given Stellar
// address
func (s *Server) getAccountDetails(w http.ResponseWriter,
	r *http.Request) {
	var request struct {
		Address string `json:"address"`
	}
	err := decodeJSONRequest(r, &request)
	if err != nil {
		log.Debugf("Error decoding JSON: %v", err)
		return
	}
	address := request.Address
	log.Debugf("Looking up balance for account with address %s", address)
	balances, err := s.yinbiClient.GetBalances(address)
	if err != nil {
		log.Debugf("Error retrieving balance: %v", err)
		return
	}
	log.Debugf("Looking up payments for account with address %s", address)
	payments, err := s.yinbiClient.GetPayments(address)
	if err != nil {
		log.Debugf("Error retrieving payments: %v", err)
		return
	}
	log.Debugf("Successfully retrived balance and payments for %s",
		address)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"balances": balances,
		"payments": payments,
	})
	return
}

// createAccountHandler is the HTTP handler used to create new
// Yinbi accounts
// First, the mnemonic is extracted from the request.
// This returns a full keypair with signing capabilities
// After the account has been created, we store the encrypted
// secret key in the key store and create a trust line to the
// Yinbi asset
func (s *Server) createAccountHandler(w http.ResponseWriter,
	r *http.Request) {
	params, pair, err := yinbi.ParseAddress(r)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	// send secret key to keystore
	err = s.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}

	onResp := func(body []byte) error {
		// after an account for the user is successfully created,
		// create a trustline to the Yinbi asset
		return s.yinbiClient.TrustAsset(issuer, pair)
	}

	err = s.proxyHandler(r, w, onResp)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
}
