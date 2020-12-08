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
	redemptionCodesEndpoint     = "/wallet/codes"
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

func (h YinbiHandler) ConfigureRoutes(r *mux.Router) http.Handler {

	log.Debug("Configuring Yinbi routes")

	accountRouter := r.PathPrefix("/account").Subrouter()
	walletRouter := r.PathPrefix("/wallet").Subrouter()
	paymentRouter := r.PathPrefix("/payment").Subrouter()
	userRouter := r.PathPrefix("/user").Subrouter()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		head, _ := h.ShiftPath(r.URL.Path)
		switch head {
		case "/payment":
			paymentRouter.ServeHTTP(w, r)
		case "/account":
			accountRouter.ServeHTTP(w, r)
		case "/user":
			userRouter.ServeHTTP(w, r)
		case "/wallet":
		default:
			walletRouter.ServeHTTP(w, r)
		}
	})
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

func (h YinbiHandler) walletHandler(w http.ResponseWriter, r *http.Request) {
	_, tail := h.ShiftPath(r.URL.Path)
	switch tail {
	case "/redeem/codes":
		url := h.GetAuthAddr(redeemCodesEndpoint)
		log.Debugf("Sending redeem codes request to %s", url)
		h.ProxyHandler(url, r, w, nil)
	case "/import":
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
	case "/codes":
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
	}
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) paymentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
}

func (h YinbiHandler) userHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, tail := h.ShiftPath(r.URL.Path)
		switch tail {
		// createAccountHandler is the HTTP handler used to create new
		// Yinbi accounts
		// First, the mnemonic is extracted from the request.
		// This returns a full keypair with signing capabilities
		// After the account has been created, we store the encrypted
		// secret key in the key store and create a trust line to the
		// Yinbi asset
		case "/account/new":
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
		case "/mnemonic":
			mnemonic := h.yinbiClient.CreateMnemonic()
			h.SuccessResponse(w, map[string]interface{}{
				"mnemonic": mnemonic,
			})
		case "/address":
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
		}
	})
}
