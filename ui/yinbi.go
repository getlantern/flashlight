package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/config"
	"github.com/getlantern/yinbi-server/crypto"
	"github.com/getlantern/yinbi-server/params"
	"github.com/getlantern/yinbi-server/yinbi"
	"github.com/stellar/go/keypair"
)

const (
	issuer = "GBAUT37VU4WN466IGBMDOOWJSIGIULAQM62WB467HS4R3TJ3IEDH3FPK"
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

// errorHandler is an error handler that takes an error or Errors and writes the
// encoded JSON response to the client
func (s *Server) errorHandler(w http.ResponseWriter, err interface{}, errorCode int) {
	var resp Response
	switch err.(type) {
	case error:
		resp.Error = err.(error).Error()
	case Errors:
		resp.Errors = err.(Errors)
	}
	common.WriteJSON(w, errorCode, &resp)
}

func (s *Server) createMnemonic(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"mnemonic": crypto.NewMnemonic(),
		"success":  true,
	})
}

// redeemCodesHandler is the handler used for redeeming yin.bi
// voucher codes
func (s *Server) redeemCodesHandler(w http.ResponseWriter,
	req *http.Request) {
	url := s.getAPIAddr(html.EscapeString(req.URL.Path))
	proxyReq, err := common.NewProxyRequest(req, url)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	r, err := s.httpClient.Do(proxyReq)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var resp ImportWalletResponse
	err = json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
}

// importWalletHandler is the handler used to import wallets
// of existing yin.bi users
func (s *Server) importWalletHandler(w http.ResponseWriter,
	req *http.Request) {
	var params AuthParams
	err := common.DecodeJSONRequest(req, &params)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))
	url := s.getAPIAddr(html.EscapeString(req.URL.Path))
	proxyReq, err := common.NewProxyRequest(req, url)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	r, err := s.httpClient.Do(proxyReq)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var resp ImportWalletResponse
	err = json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	pair, err := s.yinbiClient.DecryptSeed(resp.Seed, resp.Salt,
		params.Password)
	// send secret key to keystore
	err = s.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		s.errorHandler(w, err, http.StatusBadRequest)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (s *Server) sendPaymentHandler(w http.ResponseWriter,
	req *http.Request) {
	var params PaymentParams
	err := common.DecodeJSONRequest(req, &params)
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
	// This error means that the user entered
    // the wrong password in the confirmation field
	if err.Error() == "encrypted: decryption failed" {
		s.errorHandler(w, ErrInvalidCredentials, http.StatusOK)
		return
	}
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
		log.Debugf("Error sending payment: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
	err := common.DecodeJSONRequest(r, &request)
	if err != nil {
		log.Debugf("Error decoding JSON: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	address := request.Address
	log.Debugf("Looking up balance for account with address %s", address)
	balances, err := s.yinbiClient.GetBalances(address)
	if err != nil {
		log.Debugf("Error retrieving balance: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrived balance for %s", address)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"balances": balances,
	})
	return
}

func (s *Server) getAccountTransactions(w http.ResponseWriter,
	r *http.Request) {
	var request struct {
		Address        string `json:"address"`
		Cursor         string `json:"cursor"`
		Order          string `json:"order"`
		RecordsPerPage int    `json:"recordsPerPage"`
	}
	err := common.DecodeJSONRequest(r, &request)
	if err != nil {
		log.Debugf("Error decoding JSON: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Request decoded: %+v", request)
	address := request.Address
	cursor := request.Cursor
	order := request.Order
	recordsPerPage := request.RecordsPerPage
	log.Debugf("Looking up payments for account with address %s", address)
	payments, err := s.yinbiClient.GetPayments(address, cursor,
		order, recordsPerPage)

	if err != nil {
		log.Debugf("Error retrieving payments: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}

	// An ascending order query means that we actually want the previous page.
	// We reverse the returned payments so that they are still displayed
	// in reverse chronological order
	if order == "asc" {
		for i, j := 0, len(payments)-1; i < j; i, j = i+1, j-1 {
			payments[i], payments[j] = payments[j], payments[i]
		}
	}
	log.Debugf("Successfully retrived payments for %s",
		address)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
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
		log.Debugf("Error parsing address: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		log.Debugf("Error marshalling request: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	// send secret key to keystore
	err = s.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		log.Debugf("Error sending secret key to keystore: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}

	err = s.proxyHandler(r, w, func(body []byte) error {
		// after an account for the user is successfully created,
		// create a trustline to the Yinbi asset
		return s.yinbiClient.TrustAsset(issuer, pair)
	})
	if err != nil {
		log.Debugf("Error handling proxy: %v", err)
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
}
