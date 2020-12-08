package yinbi

import (
	"net/http"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/client"
	"github.com/gorilla/mux"
)

const (
	// account endpoints
	accountDetailsEndpoint      = "/details"
	accountTransactionsEndpoint = "/transactions"
	accountRecoverEndpoint      = "/recover"
	resetPasswordEndpoint       = "/password/reset"
)

func (h YinbiHandler) resetPassword() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// resetPasswordHandler is the handler used to reset a user's
		// wallet password
		var params api.CreateAccountParams
		err := common.DecodeJSONRequest(r, &params)
		if err != nil {
			h.ErrorHandler(w, err, http.StatusBadRequest)
			return
		}
		err = h.yinbiClient.ResetPassword(&params)
		if err != nil {
			h.ErrorHandler(w, err, http.StatusBadRequest)
		}
	})
}

func (h YinbiHandler) recoverAccount() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		h.SuccessResponse(w, map[string]interface{}{
			"user": userResponse.User,
		})
	})
}

func (h YinbiHandler) accountHandler() http.Handler {
	r := mux.NewRouter()
	r.Handle(accountDetailsEndpoint, h.getAccountDetails()).Methods("GET")
	r.Handle(resetPasswordEndpoint, h.resetPassword()).Methods("POST")
	r.Handle(accountTransactionsEndpoint, h.getAccountTransactions()).Methods("GET")
	r.Handle(accountRecoverEndpoint, h.recoverAccount()).Methods("POST")
	return r
}

// getAccountDetails is the handler used to look up
// the balances and transaction history for a given Stellar
// address
func (h YinbiHandler) getAccountDetails() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		h.SuccessResponse(w, map[string]interface{}{
			"balances": details.Balances,
		})
	})
}

func (h YinbiHandler) getAccountTransactions() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params client.AccountTransactionParams
		err := common.DecodeJSONRequest(r, &params)
		if err != nil {
			log.Debugf("Error decoding JSON: %v", err)
			h.ErrorHandler(w, err, http.StatusInternalServerError)
			return
		}
		payments, err := h.yinbiClient.GetPayments(&params)
		if err != nil {
			h.ErrorHandler(w, err, http.StatusInternalServerError)
			return
		}
		h.SuccessResponse(w, map[string]interface{}{
			"payments": payments,
		})
	})
}
