package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	// BodyKey is the context key for adding
	// the parsed body to the request context
	BodyKey = "body"
)

// BodyParser is an HTTP middleware used to
// convert the io.Reader body of an http.Request
// into a byte array
func BodyParser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			log.Errorf("Unable to read request body: %v", err)
			return
		}
		ctx := context.WithValue(req.Context(), BodyKey, bytes)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// DecodeJSONRequest extracts the bytes from the request context
// added by the BodyParser middleware. The map is then
// unmarshalled into the interface{} specified by target
func DecodeJSONRequest(w http.ResponseWriter, req *http.Request,
	target interface{}) error {
	args, ok := req.Context().Value(BodyKey).([]byte)
	if !ok || args == nil {
		err := fmt.Errorf("Unable to read request body")
		ErrorHandler(w, err, http.StatusBadRequest)
		return err
	}
	return json.Unmarshal(args, &target)
}

func JSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-type", "application/json")
		next.ServeHTTP(w, req)
	})
}
