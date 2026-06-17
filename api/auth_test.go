// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

// newTestManager creates a minimal manager.Manager for auth tests.
// It uses memory storage and does not start the manager goroutine.
func newTestManager(cfg *config.Config) *manager.Manager {
	cfg.ValidateAndClamp()
	return manager.NewManager(cfg)
}

func TestAuthMiddleware(t *testing.T) {
	// Note: no t.Parallel() here — t.Setenv is incompatible with parallel tests.
	// Subtests inherit the env vars set at parent level.

	// Clear env vars that could pollute test results.
	t.Setenv("DM_AUTH_PASSWORD", "")
	t.Setenv("DM_AUTH_TOKEN", "")

	tests := []struct {
		name       string
		authType   string
		password   string
		token      string
		setupReq   func(r *http.Request)
		wantStatus int
	}{
		{
			name:       "none passes through",
			authType:   "none",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty type passes through",
			authType:   "",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "basic valid credentials",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) { r.SetBasicAuth("admin", "secret") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "basic invalid password",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) { r.SetBasicAuth("admin", "wrong") },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "basic missing credentials",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "basic wrong username",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) { r.SetBasicAuth("hacker", "secret") },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token valid bearer",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "Bearer mytoken") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "token valid without bearer prefix",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "mytoken") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "token invalid",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token missing",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token empty rejects all",
			authType:   "token",
			token:      "",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "Bearer mytoken") },
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{
				Server: config.Server{
					WorkDir: t.TempDir(),
					Auth: config.AuthConfig{
						Type:     tc.authType,
						Password: tc.password,
						Token:    tc.token,
					},
				},
			}

			srv := NewServer(newTestManager(cfg))
			router := srv.Router()

			req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
			tc.setupReq(req)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}
