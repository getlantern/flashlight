package yinbi

import (
	"net/http"

	authapi "github.com/getlantern/auth-server/api"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/yinbi-server/api"
	"github.com/getlantern/yinbi-server/client"
	"github.com/go-chi/chi"
)

// accountHandler is the http.Handler used for handling account-related requests
func (h YinbiHandler) accountHandler() http.Handler {
	r := handler.NewRouter()
	r.Group(func(r chi.Router) {
		r.Get("/details", h.getAccountDetails)
		r.Post("/password/reset", h.resetPassword)
		r.Post("/transactions", h.updateAccountTransactions)
		r.Post("/recover", h.recoverAccount)
	})
	return r
}

// resetPassword is the http.Handler used for handling password reset requests
func (h YinbiHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	// resetPasswordHandler is the handler used to reset a user's
	// wallet password
	var params authapi.CreateAccountParams
	err := handler.GetParams(w, r, &params)
	if err != nil {
		return
	}
	pair, err := client.KeyPairFromMnemonic(params.Words)
	if err != nil {
		handler.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	resp, err := h.authClient.ResetPassword(params.UserParams)
	if err != nil {
		log.Errorf("Encountered error resetting user password: %v", err)
		handler.ErrorHandler(w, err, resp.StatusCode)
		return
	}
	log.Debug("Storing secret key in keystore with updated password")
	err = h.yinbiClient.StoreKey(pair.Seed(), params.Username, params.Password)
	if err != nil {
		log.Errorf("Error saving key to keystore with updated password: %v", err)
		handler.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debug("Successfully stored key with updated password in keystore")
	handler.HandleAuthResponse(resp, w, nil)
}

// recoverAccount is the http.Handler used for handling account recovery
// requests
func (h YinbiHandler) recoverAccount(w http.ResponseWriter, r *http.Request) {
	log.Debug("Received new recover account request")
	var params struct {
		Words string `json:"words"`
	}
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		return
	}
	resp, err := h.yinbiClient.RecoverAccount(params.Words)
	if err != nil {
		handler.ErrorHandler(w, err, resp.StatusCode)
		return
	}
	log.Debugf("Successfully recovered user %s's account using Yinbi key",
		resp.User.Username)
	handler.SuccessResponse(w, map[string]interface{}{
		"user": resp.User,
	})
}

// getAccountDetails is the handler used to look up
// the balances and transaction history for a given Stellar
// address
func (h YinbiHandler) getAccountDetails(w http.ResponseWriter, r *http.Request) {
	address := handler.GetQueryParam(r, "address")
	if address == "" {
		log.Debug("No address provided. Ignoring get account details request")
		return
	}
	log.Debugf("Looking up balance for account with address %s", address)
	details, err := h.yinbiClient.GetAccountDetails(address)
	if err != nil {
		log.Errorf("Error retrieving balance: %v", err)
		handler.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrived balance for %s", address)
	handler.SuccessResponse(w, map[string]interface{}{
		"balances": details.Balances,
	})
}

// updateAccountTransactions looks up the transaction
// history on the Stellar network for the given account
func (h YinbiHandler) updateAccountTransactions(w http.ResponseWriter, r *http.Request) {
	var params api.AccountTransactionParams
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		log.Errorf("Error decoding JSON request for account transactions: %v", err)
		handler.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debugf("Looking up payments for account with address %s", params.Address)
	resp, err := h.yinbiClient.GetPayments(&params)
	if err != nil {
		handler.ErrorHandler(w, err, resp.StatusCode)
		return
	}
	handler.SuccessResponse(w, map[string]interface{}{
		"payments": resp.Payments,
	})
}
