package yinbi

import (
	"net/http"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/yinbi-server/client"
)

const (
	// account endpoints
	accountDetailsEndpoint      = "/details"
	accountTransactionsEndpoint = "/transactions"
	accountRecoverEndpoint      = "/recover"
	resetPasswordEndpoint       = "/password/reset"
)

// accountHandler is the http.Handler used for handling account-related requests
func (h YinbiHandler) accountHandler() http.Handler {
	r := handler.NewRouter()
	r.Get(accountDetailsEndpoint, h.getAccountDetails)
	r.Post(resetPasswordEndpoint, h.resetPassword)
	r.Get(accountTransactionsEndpoint, h.getAccountTransactions)
	r.Post(accountRecoverEndpoint, h.recoverAccount)
	return r
}

// resetPassword is the http.Handler used for handling password reset requests
func (h YinbiHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	// resetPasswordHandler is the handler used to reset a user's
	// wallet password
	var params api.CreateAccountParams
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		return
	}
	err = h.yinbiClient.ResetPassword(&params)
	if err != nil {
		handler.ErrorHandler(w, err, http.StatusBadRequest)
	} else {
		handler.SuccessResponse(w, nil)
	}
}

// recoverAccount is the http.Handler used for handling account recovery
// requests
func (h YinbiHandler) recoverAccount(w http.ResponseWriter, r *http.Request) {
	var params struct {
		Words string `json:"words"`
	}
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		return
	}
	log.Debug("Received new recover account request")
	userResponse, err := h.yinbiClient.RecoverAccount(params.Words)
	if err != nil {
		handler.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	log.Debugf("Successfully recovered user %s's account using Yinbi key",
		userResponse.User.Username)
	handler.SuccessResponse(w, map[string]interface{}{
		"user": userResponse.User,
	})
}

// getAccountDetails is the handler used to look up
// the balances and transaction history for a given Stellar
// address
func (h YinbiHandler) getAccountDetails(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Address string `json:"address"`
	}
	err := handler.DecodeJSONRequest(w, r, &request)
	if err != nil {
		return
	}
	address := request.Address
	log.Debugf("Looking up balance for account with address %s", address)
	details, err := h.yinbiClient.GetAccountDetails(address)
	if err != nil {
		log.Debugf("Error retrieving balance: %v", err)
		handler.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	log.Debugf("Successfully retrived balance for %s", address)
	handler.SuccessResponse(w, map[string]interface{}{
		"balances": details.Balances,
	})

}

func (h YinbiHandler) getAccountTransactions(w http.ResponseWriter, r *http.Request) {
	var params client.AccountTransactionParams
	err := handler.DecodeJSONRequest(w, r, &params)
	if err != nil {
		return
	}
	payments, err := h.yinbiClient.GetPayments(&params)
	if err != nil {
		handler.ErrorHandler(w, err, http.StatusInternalServerError)
		return
	}
	handler.SuccessResponse(w, map[string]interface{}{
		"payments": payments,
	})
}
