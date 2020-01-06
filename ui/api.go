package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/yinbi-server/crypto"
	"github.com/getlantern/yinbi-server/yinbi"
	"github.com/stellar/go/keypair"
)

const (
	issuer = "GBR6Z4EEHLWASJY3A26IY5AE2K7B6K2F2GDZ2DW44ZWTBTEZYD4KPPA2"
)

var (
	ErrInvalidMnemonic = errors.New("The provided words do not comprise a valid mnemonic")
)

func (s *Server) createMnemonic() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		mnemonic := crypto.NewMnemonic()
		result := map[string]interface{}{
			"mnemonic": mnemonic,
			"success":  true,
		}
		err := writeJSON(w, http.StatusOK, result)
		if err != nil {
			log.Error(err)
		}
	})
}

func (s *Server) errorHandler(w http.ResponseWriter, err error, errorCode int) {
	log.Error(err)
	e := map[string]interface{}{
		"error": err.Error(),
	}
	js, err := json.Marshal(e)
	if err != nil {
		log.Error(err)
		return
	}
	w.WriteHeader(errorCode)
	w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	w.Write(js)
}

func (s *Server) sendPaymentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,
		req *http.Request) {
		var r struct {
			Password    string `json:"password"`
			Destination string `json:"address"`
			Amount      string `json:"amount"`
		}
		err := decodeJSONRequest(req, &r)
		if err != nil {
			s.errorHandler(w, err, http.StatusInternalServerError)
			return
		}
		// get key from keystore here
		secretKey, err := s.keystore.GetKey(r.Password)
		if err != nil {
			s.errorHandler(w, err, http.StatusInternalServerError)
			return
		}
		pair, err := keypair.Parse(secretKey)
		if err != nil {
			s.errorHandler(w, err, http.StatusInternalServerError)
			return
		}
		_, err = s.yinbiClient.SendPayment(
			r.Destination,
			r.Amount,
			"YNB",
			pair.(*keypair.Full),
		)
		if err != nil {
			s.errorHandler(w, err, http.StatusInternalServerError)
			return
		}
	})
}

func (s *Server) getAccountDetails() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		var request struct {
			Address string `json:"address"`
		}
		err := decodeJSONRequest(r, &request)
		if err != nil {
			log.Debugf("Error decoding JSON: %v", err)
			return
		}
		log.Debugf("Looking up balance for account with address %s", request.Address)
		balances, err := s.yinbiClient.GetBalances(request.Address)
		if err != nil {
			log.Error(err)
			return
		}
		payments, err := s.yinbiClient.GetPayments(request.Address)
		if err != nil {
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":  true,
			"balances": balances,
			"payments": payments,
		})
		return
	})
}

// createAccountHandler is the HTTP handler used to create new
// Yinbi accounts
// First, the mnemonic is extracted from the request.
// This returns a full keypair with signing capabilities
// After the account has been created, we store the encrypted
// secret key in the key store and create a trust line to the
// Yinbi asset
func (s *Server) createAccountHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,
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
		err = s.keystore.Store(pair.Seed(), params.Password)
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
	})
}
