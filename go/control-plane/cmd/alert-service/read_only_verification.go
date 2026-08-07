package main

import (
	"net/http"
)

// readOnlyVerificationHandler makes an isolated candidate incapable of
// accepting state-changing HTTP requests while its GET paths use real backing
// services. Background consumers, producers and workers are separately
// disabled by READ_ONLY_VERIFICATION_MODE in main.
func readOnlyVerificationHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"data":null,"meta":{"read_only_verification":true},"error":{"code":"READ_ONLY_VERIFICATION","message":"state-changing requests are disabled in read-only verification mode"}}`))
		}
	})
}
