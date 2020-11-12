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

	"github.com/getlantern/appdir"
	"github.com/getlantern/auth-server/models"
	. "github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ui/api"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/config"
	"github.com/getlantern/yinbi-server/crypto"
	"github.com/getlantern/yinbi-server/keystore"
	ymodels "github.com/getlantern/yinbi-server/models"
	"github.com/getlantern/yinbi-server/params"
	"github.com/getlantern/yinbi-server/yinbi"
	"github.com/stellar/go/keypair"
)

const (
	accountDetailsEndpoint      = "/account/details"
	accountTransactionsEndpoint = "/account/transactions"
	createAccountEndpoint       = "/user/account/new"
	createMnemonicEndpoint      = "/user/mnemonic"
	importEndpoint              = "/import"
	importWalletEndpoint        = "/wallet/import"
	recoverAccountEndpoint      = "/account/recover"
	redeemCodesEndpoint         = "/wallet/redeem/codes"
	resetPasswordEndpoint       = "/account/password/reset"
	saveAddressEndpoint         = "/user/address"
	sendPaymentEndpoint         = "/payment/new"
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

func (h YinbiHandler) Routes() []handler.Route {
	return []handler.Route{
		handler.Route{
			sendPaymentEndpoint,
			common.POST,
			h.sendPaymentHandler,
		},
		handler.Route{
			createAccountEndpoint,
			common.POST,
			h.createAccountHandler,
		},
		handler.Route{
			importWalletEndpoint,
			common.POST,
			h.importWalletHandler,
		},
		handler.Route{
			accountDetailsEndpoint,
			common.GET,
			h.getAccountDetails,
		},
		handler.Route{
			resetPasswordEndpoint,
			common.POST,
			h.resetPasswordHandler,
		},
		handler.Route{
			recoverAccountEndpoint,
			common.POST,
			h.recoverYinbiAccount,
		},
		handler.Route{
			accountTransactionsEndpoint,
			common.POST,
			h.getAccountTransactions,
		},
		handler.Route{
			saveAddressEndpoint,
			common.POST,
			h.saveAddressHandler,
		},
		handler.Route{
			createMnemonicEndpoint,
			common.POST,
			h.createMnemonic,
		},
		handler.Route{
			redeemCodesEndpoint,
			common.POST,
			h.redeemCodesHandler,
		},
	}
}

func New(params api.Params) YinbiHandler {
	return YinbiHandler{
		keystore:    keystore.New(appdir.General("Lantern")),
		yinbiClient: newYinbiClient(params.HttpClient),
	}
}

func NewWithAuth(params api.Params,
	authHandler auth.AuthHandler) YinbiHandler {
	return YinbiHandler{
		AuthHandler: authHandler,
		keystore:    keystore.New(appdir.General("Lantern")),
		yinbiClient: newYinbiClient(params.HttpClient),
	}
}

func newYinbiClient(httpClient *http.Client) *yinbi.Client {
	code := YinbiAssetCode
	networkName := NetworkName
	horizonAddr := HorizonAddr
	issuer := YinbiIssuerAccount
	cfg := config.GetStellarConfig(networkName, horizonAddr, issuer, code)
	return yinbi.New(params.Params{
		HttpClient: httpClient,
		Config:     cfg,
	})
}

func (s *YinbiHandler) createMnemonic(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"mnemonic": crypto.NewMnemonic(),
		"success":  true,
	})
}

func (s *YinbiHandler) redeemCodesHandler(w http.ResponseWriter, req *http.Request) {
	url := s.GetAuthAddr(redeemCodesEndpoint)
	log.Debugf("Sending redeem codes request to %s", url)
	s.ProxyHandler(url, req, w, nil)
}

// importWalletHandler is the handler used to import wallets
// of existing yin.bi users
func (h *YinbiHandler) importWalletHandler(w http.ResponseWriter,
	req *http.Request) {
	log.Debug("New import wallet request")
	url := h.GetYinbiAddr(html.EscapeString(req.URL.Path))
	var params ImportWalletParams
	err := common.DecodeJSONRequest(req, &params)
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
	pair, resp, err := h.sendImportWallet(url, &params, req)
	if err != nil {
		log.Errorf("Error sending import wallet request: %v %v", resp.Error, err)
		h.sendImportError(w, resp)
		return
	}
	// store the imported wallet in the keystore
	err = h.keystore.Store(pair.Seed(), params.Username,
		params.Password)
	if err != nil {
		log.Errorf("Error storing key in keystore: %v", err)
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debug("Successfully imported yin.bi wallet")

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"address": pair.Address(),
		"success": true,
	})
}

func (h YinbiHandler) sendImportError(w http.ResponseWriter, resp *ImportWalletResponse) {
	if resp == nil {
		return
	}
	if resp.Error != "" {
		h.ErrorHandler(w, resp.Error, http.StatusBadRequest)
	} else if len(resp.Errors) > 0 {
		h.ErrorHandler(w, resp.Errors, http.StatusBadRequest)
	}
}

func (h YinbiHandler) sendSuccess(w http.ResponseWriter) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h YinbiHandler) sendImportWallet(uri string, params *ImportWalletParams, req *http.Request) (*keypair.Full, *ImportWalletResponse, error) {
	proxyReq, err := common.NewProxyRequest(req, uri)
	if err != nil {
		return nil, nil, err
	}
	resp, err := h.HttpClient.Do(proxyReq)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	var importResp ImportWalletResponse
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &importResp, errors.New("Invalid response")
	}
	pair, err := h.decryptSeed(&importResp, params.Password)
	if err != nil {
		return nil, nil, err
	}
	return pair, &importResp, nil
}

func (h YinbiHandler) createUserAccount(w http.ResponseWriter, params *ImportWalletParams) error {
	userParams := models.UserParams{
		Email:    params.Email,
		Username: params.Username,
		Password: params.Password,
		Address:  params.Address,
	}
	log.Debugf("Sending create user account request with address %s",
		params.Address)
	srpParams, srpClient, err := h.NewSRPClient(userParams, false)
	if err != nil {
		return err
	}
	h.SendAuth(w, importEndpoint, srpClient, srpParams)
	return nil
}

func (h YinbiHandler) decryptSeed(resp *ImportWalletResponse, password string) (*keypair.Full, error) {
	pair, err := h.yinbiClient.DecryptSeed(resp.Seed, resp.Salt,
		password)
	if err != nil {
		log.Errorf("Unable to decrypt seed: %v", err)
		return nil, err
	}
	return pair, nil
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) sendPaymentHandler(w http.ResponseWriter,
	req *http.Request) {
	var params PaymentParams
	err := common.DecodeJSONRequest(req, &params)
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
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
	err := common.DecodeJSONRequest(r, &request)
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
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"balances": balances,
	})
	return
}

// resetPasswordHandler is the handler used to reset a user's
// wallet password
func (h YinbiHandler) resetPasswordHandler(w http.ResponseWriter,
	req *http.Request) {
	params, err := h.getCreateAccountParams(req)
	if err != nil {
		log.Debugf("Error parsing address: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	pair, err := yinbi.GetKeyPair(params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	srpParams, srpClient, err := h.NewSRPClient(models.UserParams{
		Username: params.Username,
		Password: params.Password,
	}, true)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	newPassword := params.Password
	params.Password = ""
	log.Debugf("Received new reset password request from %s", params.Username)
	resp, authResp, err := h.SendAuthRequest(common.POST, resetPasswordEndpoint,
		srpParams)
	if err != nil {
		log.Error(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		h.ErrorHandler(w, errors.New("Service unavailable"),
			http.StatusInternalServerError)
		return
	}
	err = h.HandleAuthResponse(srpClient, w, srpParams, authResp)
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

func (h *YinbiHandler) saveAddressHandler(w http.ResponseWriter,
	r *http.Request) {
	address := r.URL.Query().Get("address")
	url := h.GetAuthAddr(fmt.Sprintf("/user/address/%s", address))
	log.Debugf("Sending save address request to %s", url)
	h.ProxyHandler(url, r, w, nil)
}

func (h *YinbiHandler) recoverYinbiAccount(w http.ResponseWriter,
	r *http.Request) {
	var params struct {
		Words string `json:"words"`
	}
	err := common.DecodeJSONRequest(r, &params)
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
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
	resp, err := h.DoHTTPRequest(http.MethodPost, h.GetAuthAddr(recoverAccountEndpoint),
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
	err := common.DecodeJSONRequest(r, &request)
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
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"payments": payments,
	})
	return
}

// getCreateAccuntParams decodes the JSON request to create an account
func (h YinbiHandler) getCreateAccountParams(req *http.Request) (*ymodels.CreateAccountParams, error) {
	var params ymodels.CreateAccountParams
	err := common.DecodeJSONRequest(req, &params)
	if err != nil {
		return nil, err
	}
	return &params, nil
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
	log.Debug("Received new create Yinbi account request")
	params, err := h.getCreateAccountParams(r)
	if err != nil {
		log.Debugf("Error parsing address: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	pair, err := yinbi.GetKeyPair(params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
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
		log.Debugf("Trusting asset %s", assetCode)
		return h.yinbiClient.TrustAsset(assetCode, YinbiIssuerAccount, pair)
	}
	url := h.GetAuthAddr(html.EscapeString(r.URL.Path))
	h.ProxyHandler(url, r, w, onResp)
}
