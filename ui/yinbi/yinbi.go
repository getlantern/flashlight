package yinbi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getlantern/appdir"
	"github.com/getlantern/auth-server/api"
	authclient "github.com/getlantern/auth-server/client"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/client"
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
	yinbiClient *client.YinbiClient

	authClient *authclient.AuthClient
}

type ImportWalletParams = client.ImportWalletParams
type ImportWalletResponse = client.ImportWalletResponse

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

func New(params api.APIParams) YinbiHandler {
	appDir := appdir.General(params.AppName)
	httpClient := params.HTTPClient
	yinbiAddr := params.YinbiServerAddr
	authClient := authclient.New(params.AuthServerAddr, httpClient)
	return YinbiHandler{
		yinbiClient: client.New(appDir, yinbiAddr, authClient, httpClient),
		authClient:  authClient,
	}
}

func NewWithAuth(params api.APIParams,
	authHandler auth.AuthHandler) YinbiHandler {
	yinbiHandler := New(params)
	yinbiHandler.AuthHandler = authHandler
	return yinbiHandler
}

func (s *YinbiHandler) successResponse(w http.ResponseWriter, args map[string]interface{}) {
	args["success"] = true
	common.WriteJSON(w, http.StatusOK, args)
}

func (s *YinbiHandler) createMnemonic(w http.ResponseWriter, r *http.Request) {
	mnemonic := s.yinbiClient.CreateMnemonic()
	s.successResponse(w, map[string]interface{}{
		"mnemonic": mnemonic,
	})
}

func (s *YinbiHandler) redeemCodesHandler(w http.ResponseWriter, req *http.Request) {
	url := s.GetAuthAddr(redeemCodesEndpoint)
	log.Debugf("Sending redeem codes request to %s", url)
	s.ProxyHandler(url, req, w, nil)
}

func (h YinbiHandler) getWalletParams(req *http.Request) (*client.ImportWalletParams, error) {
	var params client.ImportWalletParams
	err := common.DecodeJSONRequest(req, &params)
	return &params, err
}

// importWalletHandler is the handler used to import wallets
// of existing yin.bi users
func (h *YinbiHandler) importWalletHandler(w http.ResponseWriter,
	req *http.Request) {
	log.Debug("New import wallet request")
	var params client.ImportWalletParams
	err := common.DecodeJSONRequest(req, &params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	pair, err := h.yinbiClient.ImportWallet(&params)
	if err != nil {
		log.Errorf("Error sending import wallet request: %v", err)
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"address": pair.Address(),
		"success": true,
	})
}

func (h YinbiHandler) sendSuccess(w http.ResponseWriter) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h YinbiHandler) createUserAccount(w http.ResponseWriter, params *client.ImportWalletParams) error {
	userParams := &models.UserParams{
		Email:    params.Email,
		Username: params.Username,
		Password: params.Password,
		Address:  params.Address,
	}
	log.Debugf("Sending create user account request with address %s",
		params.Address)
	_, err := h.authClient.ImportWallet(userParams)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return err
	}
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

	err = h.yinbiClient.CreateAccount(params)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusInternalServerError)
	}
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) sendPaymentHandler(w http.ResponseWriter,
	req *http.Request) {
	var params client.PaymentParams
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
	log.Debugf("Successfully retrieved keypair for %s", params.Username)
	resp, err := h.yinbiClient.SendPayment(params)
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
	details, err := h.yinbiClient.GetAccountDetails(address)
	if err != nil {
		log.Debugf("Error retrieving balance: %v", err)
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrived balance for %s", address)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"balances": details.Balances,
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
	pair, err := client.KeyPairFromMnemonic(params.Words)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	newPassword := params.Password
	_, err = h.authClient.ResetPassword(params.Username, params.Password, newPassword)
	if err != nil {
		h.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	// send secret key to keystore
	err = h.yinbiClient.StoreKey(pair.Seed(), params.Username,
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
	userResponse, err := h.yinbiClient.RecoverAccount(params.Words)
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
func (h YinbiHandler) getCreateAccountParams(req *http.Request) (*authclient.CreateAccountParams, error) {
	var params authclient.CreateAccountParams
	err := common.DecodeJSONRequest(req, &params)
	if err != nil {
		return nil, err
	}
	return &params, nil
}
