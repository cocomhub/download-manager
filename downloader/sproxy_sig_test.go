// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"

	"github.com/cocomhub/sproxy/pkg/accesskey"
	"github.com/cocomhub/sproxy/pkg/client"
	"github.com/cocomhub/sproxy/pkg/sproxysig"
)

// TestSproxyHybrid_SigAuth_Submit 验证 SproxySig 凭据配置时 submit 带合法签名头：
// mock 服务端用 sproxysig.ParseHeader + Verify 真实验签，签名无效 → 401/500。
// 这是最严格的 TDD（走真实签名链路，非仅断言头形态）。
func TestSproxyHybrid_SigAuth_Submit(t *testing.T) {
	t.Parallel()
	// 凭据（mock 服务端持 skeyID→SK 表）
	ak := "ak-test"
	sk := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	skid := "skey-1234567890ab"

	// mock 服务端：验签通过才回 200
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "SproxySig ") {
			http.Error(w, "not sproxysig", http.StatusUnauthorized)
			return
		}
		hdr, err := sproxysig.ParseHeader(auth)
		if err != nil {
			http.Error(w, "malformed: "+err.Error(), http.StatusUnauthorized)
			return
		}
		// 用 skeyID 查表（真实服务端 ring.GetEntry 语义）
		if hdr.EntryID != skid || hdr.AK != ak {
			http.Error(w, "entry mismatch", http.StatusUnauthorized)
			return
		}
		// 验签（body hash 参与：真实服务端用 bodyValidator 在 EOF 比对）
		if verr := sproxysig.Verify(sk, hdr, sproxysig.Request{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}, time.Now(), 0, 0, nil); verr != nil {
			http.Error(w, "bad sig: "+verr.Error(), http.StatusUnauthorized)
			return
		}
		writeJSONResp(w, map[string]any{"id": "task-1", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	taskID, err := d.submit("https://mypikpak.com/s/abc", "out.mp4", nil)
	if err != nil {
		t.Fatalf("submit with sig auth: %v", err)
	}
	if taskID != "task-1" {
		t.Fatalf("taskID = %q", taskID)
	}
}

// TestSproxyHybrid_SigAuth_Poll 验证轮询也带签名（GET 任务状态）。
func TestSproxyHybrid_SigAuth_Poll(t *testing.T) {
	t.Parallel()
	ak := "ak-test2"
	sk := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	skid := "skey-abcdef123456"

	var polled atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cloud/tasks/task-sig", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		hdr, err := sproxysig.ParseHeader(auth)
		if err != nil {
			http.Error(w, "malformed", http.StatusUnauthorized)
			return
		}
		if hdr.EntryID != skid || hdr.AK != ak {
			http.Error(w, "entry mismatch", http.StatusUnauthorized)
			return
		}
		if verr := sproxysig.Verify(sk, hdr, sproxysig.Request{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}, time.Now(), 0, 0, nil); verr != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		polled.Store(true)
		writeJSONResp(w, map[string]any{"id": "task-sig", "status": "completed"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       10,
	})
	if err := d.poll("task-sig"); err != nil {
		t.Fatalf("poll with sig auth: %v", err)
	}
	if !polled.Load() {
		t.Fatal("poll did not hit mock")
	}
}

// TestSproxyHybrid_RenewRotation 验证 SK 轮换：服务端拒绝当前 SK（条目过期）→
// 客户端自动 RenewAccessKey（服务端返回新 SK 信封）→ 用新 SK 重试成功。
//
// 轮换策略（用户确认）：拿到凭证记录过期时间，提前 24h 开始每小时轮换直到成功。
// 本测试验证核心链路：RenewAccessKey 调用 + 热替换后签名立即用新 SK。
func TestSproxyHybrid_RenewRotation(t *testing.T) {
	t.Parallel()
	ak := "ak-rotate"
	oldSK := "0000000000000000000000000000000000000000000000000000000000000001"
	newSK := "0000000000000000000000000000000000000000000000000000000000000002"
	oldSkid := "skey-rotate0001"
	newSkid := "skey-rotate0002"

	// 信封：服务端用旧 SK 派生 wrap key 包裹新 SK（client.RenewAccessKey 契约）
	skBytes := mustDecodeHex(t, oldSK)
	wk, err := accesskey.DeriveWrapKey(skBytes, ak, client.CredentialWrapContext(ak))
	if err != nil {
		t.Fatal(err)
	}
	env, err := accesskey.EncryptSecret(ak, mustDecodeHex(t, newSK), wk)
	if err != nil {
		t.Fatal(err)
	}

	var renewHit atomic.Bool
	mux := http.NewServeMux()
	// 任务详情：先 401（旧 SK 失效）→ 新 SK 有效
	var sigWith atomic.Int64 // 0=未验证, 1=旧SK已验证, 2=新SK已验证
	mux.HandleFunc("POST /api/credentials/ak-rotate/renew", func(w http.ResponseWriter, r *http.Request) {
		renewHit.Store(true)
		// 轮换请求用旧 SK 签名（renew 引导例外或缺 skey-id）
		writeJSONResp(w, map[string]any{
			"ak": ak, "sk_id": newSkid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			},
		})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-rot", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		hdr, err := sproxysig.ParseHeader(auth)
		if err != nil {
			http.Error(w, "malformed", http.StatusUnauthorized)
			return
		}
		var skUsed string
		switch hdr.EntryID {
		case newSkid:
			skUsed = newSK
			sigWith.Store(2)
		case oldSkid:
			skUsed = oldSK
			sigWith.Store(1)
		default:
			http.Error(w, "unknown skey", http.StatusUnauthorized)
			return
		}
		if verr := sproxysig.Verify(skUsed, hdr, sproxysig.Request{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}, time.Now(), 0, 0, nil); verr != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		// 旧 SK → 401 模拟轮换触发；新 SK → 200
		if skUsed == oldSK {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		writeJSONResp(w, map[string]any{"id": "task-rot", "status": "completed"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: oldSK,
		AccessKeyID:     oldSkid,
		PollEvery:       10,
	})
	// 场景：poll 先遇 401（旧 SK 失效）→ 触发 renew → 热替换 → 重试成功
	err = d.pollWithRotate("task-rot")
	if err != nil {
		t.Fatalf("poll with rotate: %v", err)
	}
	if !renewHit.Load() {
		t.Fatal("renew not triggered")
	}
	if sigWith.Load() != 2 {
		t.Fatalf("final signature used skid state = %d, want 2 (new SK)", sigWith.Load())
	}
}

// mustDecodeHex 解码 hex（测试 helper）。
func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi := hexVal(s[i])
		lo := hexVal(s[i+1])
		b[i/2] = hi<<4 | lo
	}
	return b
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// TestSproxyHybrid_BearerFallback 验证旧 Bearer 配置零回归（未配 SproxySig → 仍走 Bearer）。
func TestSproxyHybrid_BearerFallback(t *testing.T) {
	t.Parallel()
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeJSONResp(w, map[string]any{"id": "task-b", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:   srv.URL + "/api/cloud/download",
		APIToken: "old-bearer-token",
	})
	if _, err := d.submit("https://mypikpak.com/s/abc", "out.mp4", nil); err != nil {
		t.Fatalf("submit bearer: %v", err)
	}
	if gotAuth != "Bearer old-bearer-token" {
		t.Fatalf("Authorization = %q, want Bearer fallback", gotAuth)
	}
}

var _ = json.Marshal // 保持 json import（后续轮换测试用）
var _ = model.DownloadObject{}

// TestSproxyHybrid_RotationSchedule 验证主动轮换调度：凭证距到期 <24h 时
// 提交前触发轮换（每小时限频）；到期>24h 时不轮换。
func TestSproxyHybrid_RotationSchedule(t *testing.T) {
	t.Parallel()
	ak := "ak-sched"
	oldSK := "1111111111111111111111111111111111111111111111111111111111111111"
	newSK := "2222222222222222222222222222222222222222222222222222222222222222"
	oldSkid := "skey-sched0001"
	newSkid := "skey-sched0002"

	skBytes := mustDecodeHex(t, oldSK)
	wk, err := accesskey.DeriveWrapKey(skBytes, ak, client.CredentialWrapContext(ak))
	if err != nil {
		t.Fatal(err)
	}
	env, err := accesskey.EncryptSecret(ak, mustDecodeHex(t, newSK), wk)
	if err != nil {
		t.Fatal(err)
	}

	var renewCount atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/credentials/ak-sched/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		writeJSONResp(w, map[string]any{
			"ak": ak, "sk_id": newSkid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			},
		})
	})
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-sched", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: oldSK,
		AccessKeyID:     oldSkid,
	})
	// 未记录到期时间 → 不会提前轮换（初次 renew 仅 401 时）
	// 模拟：凭证刚 renew 过（30 天后到期）→ 不应触发轮换
	d.expireAt = time.Now().Add(30 * 24 * time.Hour)
	d.lastRotate = time.Now()
	if err := d.ensureRotatedBeforeSubmit(); err != nil {
		t.Fatalf("ensureRotated: %v", err)
	}
	if renewCount.Load() != 0 {
		t.Fatalf("renew called = %d, want 0 (30d to expiry)", renewCount.Load())
	}
	// 模拟：距到期 23h → 触发轮换（提前 24h 窗口内）
	d.expireAt = time.Now().Add(23 * time.Hour)
	d.lastRotate = time.Now().Add(-2 * time.Hour)
	if err := d.ensureRotatedBeforeSubmit(); err != nil {
		t.Fatalf("ensureRotated near expiry: %v", err)
	}
	if renewCount.Load() != 1 {
		t.Fatalf("renew called = %d, want 1 (within 24h window)", renewCount.Load())
	}
	// 轮换成功后 1h 内不再轮换（限频）
	if err := d.ensureRotatedBeforeSubmit(); err != nil {
		t.Fatalf("ensureRotated repeat: %v", err)
	}
	if renewCount.Load() != 1 {
		t.Fatalf("renew called = %d, want still 1 (rate-limited 1h)", renewCount.Load())
	}
}
