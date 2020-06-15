package yinbi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/getlantern/appdir"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/flashlight/ui/handlers"
	"github.com/getlantern/golog"
	scommon "github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/config"
	"github.com/getlantern/yinbi-server/crypto"
	"github.com/getlantern/yinbi-server/keystore"
	yparams "github.com/getlantern/yinbi-server/params"
	"github.com/getlantern/yinbi-server/yinbi"
	"github.com/stellar/go/keypair"
)

const (
	issuer                 = "GDVT32BZETHUQGGEVOEQBSADVT4Z7F6DDBUOXUVATRHRFTT6J7RDOS76"
	recoverAccountEndpoint = "/account/recover"
	resetPasswordEndpoint  = "/account/password/reset"
)

var (
	ErrInvalidMnemonic = errors.New("The provided words do not comprise a valid mnemonic")
	log                = golog.LoggerFor("flashlight.ui.yinbi")
)

// YinbiHandler is the group of handlers used for handling
// yinbi-related requests to the UI server
type YinbiHandler struct {
	auth.AuthHandler

	// yinbiClient is a client for the Yinbi API which
	// supports creating accounts and making payments
	yinbiClient *yinbi.Client

	// keystore manages encrypted storage of Yinbi private keys
	keystore *keystore.Keystore

	proxy *httputil.ReverseProxy
}

func New(params handlers.Params) YinbiHandler {
	return YinbiHandler{
		keystore:    keystore.New(appdir.General("Lantern")),
		yinbiClient: newYinbiClient(params.HttpClient),
	}
}

func NewWithAuth(params handlers.Params,
	authHandler auth.AuthHandler) YinbiHandler {
	u, err := url.Parse(params.YinbiServerAddr)
	if err != nil {
		log.Fatal(fmt.Errorf("Bad Yinbi server address: %s", params.AuthServerAddr))
	}
	return YinbiHandler{
		AuthHandler: authHandler,
		keystore:    keystore.New(appdir.General("Lantern")),
		yinbiClient: newYinbiClient(params.HttpClient),
		proxy:       httputil.NewSingleHostReverseProxy(u),
	}
}

func (h YinbiHandler) Routes() map[string]handlers.HandlerFunc {
	proxyHandler := func(w http.ResponseWriter, r *http.Request) {
		h.proxy.ServeHTTP(w, r)
	}
	return map[string]handlers.HandlerFunc{
		"/payment/new":            h.sendPaymentHandler,
		"/user/account/new":       h.createAccountHandler,
		"/wallet/import":          h.importWalletHandler,
		"/account/details":        h.getAccountDetails,
		"/account/password/reset": h.resetPasswordHandler,
		"/account/recover":        h.recoverYinbiAccount,
		"/account/transactions":   h.getAccountTransactions,
		"/user/mnemonic":          h.createMnemonic,
		"/wallet/redeem/codes":    proxyHandler,
	}
}

func newYinbiClient(httpClient *http.Client) *yinbi.Client {
	code := "YNB"
	networkName := "test"
	horizonAddr := "https://horizon-testnet.stellar.org"
	issuer := "GDVT32BZETHUQGGEVOEQBSADVT4Z7F6DDBUOXUVATRHRFTT6J7RDOS76"
	if !common.Staging {
		networkName = "public"
		code = "Yinbi"
		horizonAddr = "https://horizon.stellar.org"
		issuer = "GDTFHBTWLOYSMX54QZKTWWKFHAYCI3NSZADKY3M7PATARUUKVWOAEY2E"
	}
	cfg := config.GetStellarConfig(networkName, horizonAddr, issuer, code)
	return yinbi.New(yparams.Params{
		HttpClient: httpClient,
		Config:     cfg,
	})
}

func (s YinbiHandler) createMnemonic(w http.ResponseWriter, r *http.Request) {
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"mnemonic": crypto.NewMnemonic(),
		"success":  true,
	})
}

// importWalletHandler is the handler used to import wallets
// of existing yin.bi users
func (h YinbiHandler) importWalletHandler(w http.ResponseWriter,
	req *http.Request) {
	var params AuthParams
	log.Debug("New import wallet request")
	err := scommon.DecodeJSONRequest(req, &params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))
	url := h.GetYinbiAddr(html.EscapeString(req.URL.Path))
	proxyReq, err := scommon.NewProxyRequest(req, url)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	r, err := h.HttpClient.Do(proxyReq)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var resp ImportWalletResponse
	err = json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	pair, err := h.yinbiClient.DecryptSeed(resp.Seed, resp.Salt,
		params.Password)
	// send secret key to keystore
	err = h.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debug("Successfully imported yin.bi wallet")
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) sendPaymentHandler(w http.ResponseWriter,
	req *http.Request) {
	var params PaymentParams
	err := scommon.DecodeJSONRequest(req, &params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	errors := params.Validate()
	if len(errors) > 0 {
		h.ErrorHandler(w, errors, http.StatusBadRequest)
		return
	}
	// get key from keystore here
	secretKey, err := h.keystore.GetKey(params.Username, params.Password)
	if err != nil {
		err = fmt.Errorf("Error retrieving secret key: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	// create a new keypair from the secret key
	// once we have the keypair, we can sign transactions
	pair, err := keypair.Parse(secretKey)
	if err != nil {
		err = fmt.Errorf("Error parsing keypair: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrieved keypair for %s", params.Username)
	resp, err := h.yinbiClient.SendPayment(
		params.Destination,
		params.Amount,
		params.Asset,
		pair.(*keypair.Full),
	)
	if err != nil {
		log.Debugf("Error sending payment: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"tx_id":   resp.Hash,
	})
	return
}

// getAccountDetails is the handler used to look up
// the balances and transaction history for a given Stellar
// address
func (h YinbiHandler) getAccountDetails(w http.ResponseWriter,
	r *http.Request) {
	var request struct {
		Address string `json:"address"`
	}
	err := scommon.DecodeJSONRequest(r, &request)
	if err != nil {
		log.Debugf("Error decoding JSON: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	address := request.Address
	log.Debugf("Looking up balance for account with address %s", address)
	balances, err := h.yinbiClient.GetBalances(address)
	if err != nil {
		log.Debugf("Error retrieving balance: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrived balance for %s", address)
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"balances": balances,
	})
	return
}

func (h YinbiHandler) resetPasswordHandler(w http.ResponseWriter,
	req *http.Request) {
	addressParams, pair, err := yinbi.ParseAddress(req)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	params, srpClient, err := h.NewSRPClient(models.UserParams{
		Username: addressParams.Username,
		Password: addressParams.Password,
	}, true)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	newPassword := params.Password
	params.Password = ""
	log.Debugf("Received new reset password request from %s", params.Username)
	resp, authResp, err := h.SendAuthRequest(scommon.POST, resetPasswordEndpoint, params)
	if err != nil {
		log.Error(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		h.ErrorHandler(w, errors.New("Service unavailable"), http.StatusInternalServerError)
		return
	}
	err = h.HandleAuthResponse(srpClient, w, params, authResp)
	if err != nil {
		return
	}
	// send secret key to keystore
	err = h.keystore.Store(pair.Seed(), params.Username,
		newPassword)
	if err != nil {
		log.Debugf("Error sending secret key to keystore: %v", err)
		return
	}
}

func (h YinbiHandler) recoverYinbiAccount(w http.ResponseWriter,
	r *http.Request) {
	var params struct {
		Words string `json:"words"`
	}
	err := scommon.DecodeJSONRequest(r, &params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debug("Received new recover account request")
	address, err := h.yinbiClient.IsMnemonicValid(params.Words)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	userResponse, err := h.sendRecoverRequest(address.Address)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debugf("Successfully recovered user %s's account using Yinbi key",
		userResponse.User.Username)
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user":    userResponse.User,
	})
}

func (h YinbiHandler) sendRecoverRequest(address string) (*models.UserResponse, error) {
	params := &models.AccountParams{
		PublicKey: address,
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	resp, err := h.DoRequest(http.MethodPost, h.GetAuthAddr(recoverAccountEndpoint),
		requestBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	userResponse := new(models.UserResponse)
	err = json.Unmarshal(body, userResponse)
	if err != nil {
		return nil, err
	}
	return userResponse, nil
}

func (h YinbiHandler) getAccountTransactions(w http.ResponseWriter,
	r *http.Request) {
	var request struct {
		Address        string `json:"address"`
		Cursor         string `json:"cursor"`
		Order          string `json:"order"`
		RecordsPerPage int    `json:"recordsPerPage"`
	}
	err := scommon.DecodeJSONRequest(r, &request)
	if err != nil {
		log.Debugf("Error decoding JSON: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	address := request.Address
	cursor := request.Cursor
	order := request.Order
	recordsPerPage := request.RecordsPerPage
	log.Debugf("Looking up payments for account with address %s", address)
	payments, err := h.yinbiClient.GetPayments(address, cursor,
		order, recordsPerPage)

	if err != nil {
		log.Debugf("Error retrieving payments: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
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
	scommon.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
func (h YinbiHandler) createAccountHandler(w http.ResponseWriter,
	r *http.Request) {
	params, pair, err := yinbi.ParseAddress(r)
	if err != nil {
		log.Debugf("Error parsing address: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		log.Debugf("Error marshalling request: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	// send secret key to keystore
	err = h.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		log.Debugf("Error sending secret key to keystore: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	onResp := func(resp *http.Response) error {
		assetCode := h.yinbiClient.GetAssetCode()
		return h.yinbiClient.TrustAsset(assetCode, issuer, pair)
	}
	url := h.GetYinbiAddr(html.EscapeString(r.URL.Path))
	err = h.ProxyHandler(url, r, w, onResp)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
	}
}
