package yinbi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getlantern/appdir"
	"github.com/getlantern/auth-server/api"
	authclient "github.com/getlantern/auth-server/client"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/getlantern/yinbi-server/client"
	"github.com/go-chi/chi"
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

// ConfigureRoutes returns an http.Handler for the Yinbi-related routes
func (h YinbiHandler) ConfigureRoutes() http.Handler {

	log.Debug("Configuring Yinbi routes")

	routes := map[string]func() http.Handler{
		"/account": h.accountHandler,
		"/":        h.walletHandler,
		"/payment": h.paymentHandler,
		"/user":    h.userHandler,
	}

	r := handler.NewRouter()
	r.Group(func(cr chi.Router) {
		for endpoint, handler := range routes {
			cr.Mount(endpoint, handler())
		}
	})

	return r
}

func (h YinbiHandler) GetPathPrefix() string {
	return "/wallet"
}

// New creates a new YinbiHandler instance
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

// walletHandler is the http.Handler used for Yinbi wallet related
// UI routes
func (h YinbiHandler) walletHandler() http.Handler {
	router := handler.NewRouter()
	router.Group(func(r chi.Router) {
		// redeemCodes
		r.Get("/redeem/codes", func(w http.ResponseWriter, r *http.Request) {
			url := h.GetAuthAddr("/redeem/codes")
			log.Debugf("Sending redeem codes request to %s", url)
			h.ProxyHandler(url, r, w, nil)
		})
		// importWallet
		r.Post("/import", func(w http.ResponseWriter, r *http.Request) {
			// importWalletHandler is the handler used to import wallets
			// of existing yin.bi users
			var params client.ImportWalletParams
			err := handler.DecodeJSONRequest(w, r, &params)
			if err != nil {
				return
			}
			resp, err := h.yinbiClient.ImportWallet(&params)
			if err != nil {
				log.Errorf("Error sending import wallet request: %v", err)
				handler.ErrorHandler(w, err, resp.StatusCode)
				return
			}
			handler.SuccessResponse(w, map[string]interface{}{
				"address": resp.Pair.Address(),
			})
		})
		// fetch redemption codes
		r.Get("/codes", func(w http.ResponseWriter, r *http.Request) {
			// getRedemptionCodes is the handler used to look up
			// voucher codes belonging to a Yinbi user
			log.Debugf("Looking up redemption codes")
			codes, err := h.yinbiClient.GetRedemptionCodes()
			if err != nil {
				log.Debugf("Error retrieving codes: %v", err)
				handler.ErrorHandler(w, err, http.StatusInternalServerError)
				return
			}
			log.Debugf("Successfully retrived %d voucher codes", len(codes))
			handler.SuccessResponse(w, map[string]interface{}{
				"codes": codes,
			})
		})

	})
	return router
}

// sendPaymentHandler is the handler used to create Yinbi payments
// The password included with the request is used to look up the
// user's secret key in the Yinbi keystore.
func (h YinbiHandler) sendPaymentHandler(w http.ResponseWriter, r *http.Request) {
	var params client.PaymentParams
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		return
	}
	errors := params.Validate()
	if len(errors) > 0 {
		handler.ErrorHandler(w, errors, http.StatusBadRequest)
		return
	}
	log.Debugf("Successfully retrieved keypair for %s", params.Username)
	resp, err := h.yinbiClient.SendPayment(params)
	if err != nil {
		log.Debugf("Error sending payment: %v", err)
		handler.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	handler.SuccessResponse(w, map[string]interface{}{
		"tx_id": resp.Hash,
	})
}

// paymentHandler setups Yinbi payment-related routes
func (h YinbiHandler) paymentHandler() http.Handler {
	paymentRouter := handler.NewRouter()
	paymentRouter.Post("/new", h.sendPaymentHandler)
	return paymentRouter
}

func (h YinbiHandler) createAccountHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("Received new create Yinbi account request")
		var params api.CreateAccountParams
		err := handler.DecodeJSONRequest(w, r, &params)
		if err != nil {
			err = fmt.Errorf("Error decoding create account request: %v", err)
			handler.ErrorHandler(w, err, http.StatusInternalServerError)
			return
		}
		resp, err := h.yinbiClient.CreateAccount(&params)
		if err != nil {
			handler.ErrorHandler(w, err, http.StatusInternalServerError)
		} else {
			handler.SuccessResponse(w, resp)
		}
	})
}

// userHandler is the http.Handler used for Yinbi wallet user related
// UI routes
func (h YinbiHandler) userHandler() http.Handler {
	r := handler.NewRouter()

	// createAccountHandler is the HTTP handler used to create new
	// Yinbi accounts
	// First, the mnemonic is extracted from the request.
	// This returns a full keypair with signing capabilities
	// After the account has been created, we store the encrypted
	// secret key in the key store and create a trust line to the
	// Yinbi asset

	r.Group(func(r chi.Router) {
		r.Post("/account/new", h.createAccountHandler())

		r.Post("/mnemonic", func(w http.ResponseWriter, r *http.Request) {
			mnemonic := h.yinbiClient.CreateMnemonic()
			handler.SuccessResponse(w, map[string]interface{}{
				"mnemonic": mnemonic,
			})
		})

		r.Post("/address", func(w http.ResponseWriter, r *http.Request) {
			// saveAddressHandler is the handler used to save new account
			// addresses for the given user
			address := r.URL.Query().Get("address")
			_, err := h.authClient.SaveAddress(address)
			if err != nil {
				err = fmt.Errorf("Error saving user address: %v", err)
				handler.ErrorHandler(w, err, http.StatusBadRequest)
			} else {
				log.Debug("Successfully saved address")
				handler.SuccessResponse(w)
			}
		})

	})
	return r
}
