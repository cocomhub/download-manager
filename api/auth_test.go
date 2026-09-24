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

func TestTokenAuthExpired(t *testing.T) {
	t.Setenv("DM_AUTH_TOKEN", "")
	cfg := config.AuthConfig{Type: "token", Token: "abc", ExpiresAt: "2020-01-01T00:00:00Z"}
	if validateTokenAuth(cfg, "Bearer abc") {
		t.Fatal("expired token should be rejected")
	}
}

func TestTokenAuthNotExpired(t *testing.T) {
	t.Setenv("DM_AUTH_TOKEN", "")
	cfg := config.AuthConfig{Type: "token", Token: "abc", ExpiresAt: "2099-01-01T00:00:00Z"}
	if !validateTokenAuth(cfg, "Bearer abc") {
		t.Fatal("non-expired token should be accepted")
	}
}

func TestTokenAuthNoExpiry(t *testing.T) {
	t.Setenv("DM_AUTH_TOKEN", "")
	cfg := config.AuthConfig{Type: "token", Token: "abc"}
	if !validateTokenAuth(cfg, "Bearer abc") {
		t.Fatal("token without expiry should be accepted")
	}
}

func TestTokenAuthInvalidExpiryFormat(t *testing.T) {
	t.Setenv("DM_AUTH_TOKEN", "")
	cfg := config.AuthConfig{Type: "token", Token: "abc", ExpiresAt: "not-a-date"}
	if !validateTokenAuth(cfg, "Bearer abc") {
		t.Fatal("unparseable expiry should not reject (fail-open on malformed value)")
	}
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

// TestAuth_HealthzExempt 验证健康检查端点豁免鉴权（容器 healthcheck 场景）。
// P3-3 依赖此行为：docker-compose 的 healthcheck 无凭据也能探活。
func TestAuth_HealthzExempt(t *testing.T) {
	cfg := &config.Config{
		Server: config.Server{
			WorkDir: t.TempDir(),
			Auth: config.AuthConfig{
				Type:     "basic",
				Username: "admin",
				Password: "secret",
			},
		},
	}
	srv := NewServer(newTestManager(cfg))
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("healthz should be exempt from auth, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}
