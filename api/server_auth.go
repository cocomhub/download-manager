// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
)

// authVerifyHandler validates the request's credentials (if auth is enabled)
// and returns 200/401 accordingly. It is exempt from authMiddleware so the
// frontend can pre-validate typed credentials before storing them.
func (s *Server) authVerifyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(hdrContentType, "application/json")
	cfg := s.mgr.GetConfig()
	authEnabled := cfg != nil && cfg.Server.Auth.Type != "" && cfg.Server.Auth.Type != "none"
	if !authEnabled || cfg == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"auth":    false,
			"message": "authorized",
		})
		return
	}
	ac := cfg.Server.Auth
	valid := false
	switch ac.Type {
	case "basic":
		user, pass, ok := r.BasicAuth()
		valid = ok && validateBasicAuth(ac, user, pass)
	case "token":
		valid = validateTokenAuth(ac, r.Header.Get("Authorization"))
	}
	if !valid {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"auth":    true,
		"message": "authorized",
	})
}
