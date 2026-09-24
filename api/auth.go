// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/gorilla/mux"
)

// authMiddleware returns an HTTP middleware that enforces the configured
// authentication scheme. It runs before writeMiddleware so that auth failures
// return 401 before write-protection checks.
func (s *Server) authMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 静态资源豁免鉴权：Web UI 的 HTML/JS/CSS 必须可公开加载，
			// 否则登录页本身都无法渲染（401 拦截 / 及其子资源）。
			// 仅 /api/ 与 /files/ 受鉴权保护。
			if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/files/") {
				next.ServeHTTP(w, r)
				return
			}
			// 健康检查端点豁免鉴权：探活不应因凭据缺失而失败（运维/容器 healthcheck 场景）。
			// /api/runtime 豁免：前端需在无凭据时探测 auth 开关状态
			// （未登录时 UI 必须能读到 auth.enabled 才能决定是否显示登录页）。
			// /api/auth/verify 也豁免：其 handler 自行校验凭据并返回 401/200，
			// 前端用它对输入的凭据做预验证（此时可能还没存任何凭据）。
			if r.URL.Path == "/api/healthz" || r.URL.Path == "/api/runtime" || r.URL.Path == "/api/auth/verify" {
				next.ServeHTTP(w, r)
				return
			}
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
	if cfg.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, cfg.ExpiresAt)
		if err == nil && time.Now().After(exp) {
			return false // token has expired
		}
	}
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
