package sms

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// HTTPHandler is a testing/integration ingress, NOT a carrier SMS webhook.
// A provider adapter must validate the provider signature before creating
// Inbound; requiring a bearer token here is not a substitute for that check.
func HTTPHandler(handler Handler, bearerToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /inbound", func(w http.ResponseWriter, r *http.Request) {
		if bearerToken != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(bearerToken)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var in Inbound
		if err := decoder.Decode(&in); err != nil {
			http.Error(w, "invalid message JSON", http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			http.Error(w, "extra message JSON", http.StatusBadRequest)
			return
		}
		out, err := handler.Handle(r.Context(), in)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrUpstream) {
				status = http.StatusBadGateway
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(struct {
			Outbound []Outbound `json:"outbound"`
		}{Outbound: out})
	})
	return mux
}
