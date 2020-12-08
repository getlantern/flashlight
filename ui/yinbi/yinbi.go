package yinbi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getlantern/appdir"
	"github.com/getlantern/auth-server/api"
	authclient "github.com/getlantern/auth-server/client"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/client"
	"github.com/gorilla/mux"
)

const (
	accountEndpoint = "/account"
	paymentEndpoint = "/payment"
	userEndpoint    = "/user"
	walletEndpoint  = "/wallet"

	importEndpoint      = "/import"
	saveAddressEndpoint = "/address"

	// wallet endpoints
	redeemCodesEndpoint     = "/redeem/codes"
	redemptionCodesEndpoint = "/codes"

	// user endpoints
	createAccountEndpoint  = "/account/new"
	createMnemonicEndpoint = "/mnemonic"

	importWalletEndpoint = "/wallet/import"

	// payment endpoints
	sendPaymentEndpoint = "/new"
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

func (h YinbiHandler) ConfigureRoutes() http.Handler {

	log.Debug("Configuring Yinbi routes")

	yinbiRouter := mux.NewRouter()

	yinbiRouter.PathPrefix(accountEndpoint).Subrouter().Handle("/", h.accountHandler())
	yinbiRouter.PathPrefix(walletEndpoint).Subrouter().Handle("/", h.walletHandler())
	yinbiRouter.PathPrefix(paymentEndpoint).Subrouter().Handle("/", h.paymentHandler())
	yinbiRouter.PathPrefix(userEndpoint).Subrouter().Handle("/", h.userHandler())
	return yinbiRouter
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

func (h YinbiHandler) walletHandler() http.Handler {

	r := mux.NewRouter()

	r.HandleFunc(redeemCodesEndpoint, func(w http.ResponseWriter, r *http.Request) {
		url := h.GetAuthAddr(redeemCodesEndpoint)
		log.Debugf("Sending redeem codes request to %s", url)
		h.ProxyHandler(url, r, w, nil)
	})

	r.HandleFunc(importEndpoint, func(w http.ResponseWriter, r *http.Request) {
		// importWalletHandler is the handler used to import wallets
		// of existing yin.bi users
		var params client.ImportWalletParams
		err := common.DecodeJSONRequest(r, &params)
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
		h.SuccessResponse(w, map[string]interface{}{
			"address": pair.Address(),
		})
	})

	r.HandleFunc(redemptionCodesEndpoint, func(w http.ResponseWriter, r *http.Request) {
		// getRedemptionCodes is the handler used to look up
		// voucher codes belonging to a Yinbi user
		log.Debugf("Looking up redemption codes")
		codes, err := h.yinbiClient.GetRedemptionCodes()
		if err != nil {
			log.Debugf("Error retrieving codes: %v", err)
			h.ErrorHandler(w, err, http.StatusInternalServerError)
			return
		}
		log.Debugf("Successfully retrived %d voucher codes", len(codes))
	})

	return r
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) paymentHandler() http.Handler {
	paymentRouter := mux.NewRouter()
	paymentRouter.HandleFunc(sendPaymentEndpoint, func(w http.ResponseWriter, r *http.Request) {
		var params client.PaymentParams
		err := common.DecodeJSONRequest(r, &params)
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
		h.SuccessResponse(w, map[string]interface{}{
			"tx_id": resp.Hash,
		})
	}).Methods("POST")
	return paymentRouter
}

func (h YinbiHandler) userHandler() http.Handler {
	r := mux.NewRouter()
	// createAccountHandler is the HTTP handler used to create new
	// Yinbi accounts
	// First, the mnemonic is extracted from the request.
	// This returns a full keypair with signing capabilities
	// After the account has been created, we store the encrypted
	// secret key in the key store and create a trust line to the
	// Yinbi asset
	r.HandleFunc(createAccountEndpoint, func(w http.ResponseWriter, r *http.Request) {
		log.Debug("Received new create Yinbi account request")
		var params api.CreateAccountParams
		err := common.DecodeJSONRequest(r, &params)
		if err != nil {
			return
		}
		err = h.yinbiClient.CreateAccount(&params)
		if err != nil {
			h.ErrorHandler(w, err, http.StatusInternalServerError)
		}
	}).Methods("POST")
	r.HandleFunc(createMnemonicEndpoint, func(w http.ResponseWriter, r *http.Request) {
		mnemonic := h.yinbiClient.CreateMnemonic()
		h.SuccessResponse(w, map[string]interface{}{
			"mnemonic": mnemonic,
		})
	}).Methods("POST")
	r.HandleFunc(saveAddressEndpoint, func(w http.ResponseWriter, r *http.Request) {
		// saveAddressHandler is the handler used to save new account
		// addresses for the given user
		address := r.URL.Query().Get("address")
		_, err := h.authClient.SaveAddress(address)
		if err != nil {
			err = fmt.Errorf("Error saving user address: %v", err)
			h.ErrorHandler(w, err, http.StatusBadRequest)
			return
		}
		log.Debug("Successfully saved address")
	}).Methods("POST")
	r.Handle(accountTransactionsEndpoint, h.getAccountTransactions()).Methods("GET")
	r.Handle(accountRecoverEndpoint, h.recoverAccount()).Methods("POST")
	return r
}
