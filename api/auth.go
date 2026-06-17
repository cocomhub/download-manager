// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/cocomhub/download-manager/config"
	"github.com/gorilla/mux"
)

// authMiddleware returns an HTTP middleware that enforces the configured
// authentication scheme. It runs before writeMiddleware so that auth failures
// return 401 before write-protection checks.
func (s *Server) authMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := s.mgr.GetConfig()
			if cfg == nil {
				next.ServeHTTP(w, r)
				return
			}
			ac := cfg.Server.Auth

			switch ac.Type {
			case "basic":
				user, pass, ok := r.BasicAuth()
				if !ok || !validateBasicAuth(ac, user, pass) {
					w.Header().Set("WWW-Authenticate", `Basic realm="download-manager"`)
					writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
					return
				}
			case "token":
				token := r.Header.Get("Authorization")
				if !validateTokenAuth(ac, token) {
					writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
					return
				}
			}
			// case "none" or empty — pass through
			next.ServeHTTP(w, r)
		})
	}
}

func validateBasicAuth(cfg config.AuthConfig, user, pass string) bool {
	expectedUser := cfg.Username
	if expectedUser == "" {
		expectedUser = "admin"
	}
	expectedPass := cfg.Password
	if envPass := os.Getenv("DM_AUTH_PASSWORD"); envPass != "" {
		expectedPass = envPass
	}
	return subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) == 1
}

func validateTokenAuth(cfg config.AuthConfig, token string) bool {
	expected := cfg.Token
	if envToken := os.Getenv("DM_AUTH_TOKEN"); envToken != "" {
		expected = envToken
	}
	if expected == "" {
		return false // token mode requires a non-empty token
	}
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
