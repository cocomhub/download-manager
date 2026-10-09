// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	mockCredList(mux, "ak-test", "skey-1234567890ab")

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	taskID, err := d.submit(context.Background(), "https://mypikpak.com/s/abc", "out.mp4", nil)
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
	mockCredList(mux, "ak-test2", "skey-abcdef123456")
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       10,
	})
	if err := d.poll(context.Background(), "task-sig"); err != nil {
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
	mockCredList(mux, "ak-rotate", "skey-rotate0001")
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
	_, rerr := d.pollWithRotateResult(context.Background(), "task-rot")
	if rerr != nil {
		t.Fatalf("poll with rotate: %v", rerr)
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
	if _, err := d.submit(context.Background(), "https://mypikpak.com/s/abc", "out.mp4", nil); err != nil {
		t.Fatalf("submit bearer: %v", err)
	}
	if gotAuth != "Bearer old-bearer-token" {
		t.Fatalf("Authorization = %q, want Bearer fallback", gotAuth)
	}
}

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
	mockCredList(mux, "ak-sched", "skey-sched0001")

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

// TestSproxyHybrid_TransferAndPullback 验证默认转存 + 可选拉回：
//  1. TransferVolume 配置 → submit 带 transfer 选项（mock 收到 body. transfer.volume）
//  2. 完成后 TransferURL 写入 obj.Extra[transfer_url]
//  3. PullBackToSavePath=true → 拉回原始文件到 SavePath（kind=cloud_task 下载）
func TestSproxyHybrid_TransferAndPullback(t *testing.T) {
	t.Parallel()
	ak := "ak-transfer"
	sk := "3333333333333333333333333333333333333333333333333333333333333333"
	skid := "skey-transfer01"

	var gotTransferVolume string
	var downloadHit atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
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
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if tr, ok := body["transfer"].(map[string]any); ok {
			gotTransferVolume, _ = tr["volume"].(string)
		}
		writeJSONResp(w, map[string]any{"id": "task-t", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://default/cloud/task-t/movie.mp4"})
	})
	// 轮询：返回 completed + transfer_url（含验签）
	mux.HandleFunc("GET /api/cloud/tasks/task-t", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		hdr, err := sproxysig.ParseHeader(auth)
		if err != nil || hdr.EntryID != skid || hdr.AK != ak {
			http.Error(w, "unauth", http.StatusUnauthorized)
			return
		}
		if verr := sproxysig.Verify(sk, hdr, sproxysig.Request{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}, time.Now(), 0, 0, nil); verr != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		writeJSONResp(w, map[string]any{"id": "task-t", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://default/cloud/task-t/movie.mp4"})
	})
	// 拉回 stat（HEAD /api/files/stat?filename=<taskID>/<file>&kind=cloud_task）
	mux.HandleFunc("HEAD /api/files/stat", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "cloud_task" || r.URL.Query().Get("filename") != "task-t/movie.mp4" {
			http.Error(w, "wrong stat params", http.StatusBadRequest)
			return
		}
		w.Header().Set("X-File-Size", "9")
		w.Header().Set("X-File-Checksum", "")
		w.WriteHeader(http.StatusOK)
	})
	// 拉回：kind=cloud_task 下载 filename=<taskID>/<file>
	mux.HandleFunc("GET /download/chunk", func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("filename")
		kind := r.URL.Query().Get("kind")
		if kind != "cloud_task" || filename != "task-t/movie.mp4" {
			http.Error(w, "wrong download params", http.StatusBadRequest)
			return
		}
		downloadHit.Store(true)
		w.Header().Set("Content-Range", "bytes 0-8/9")
		w.Write([]byte("fakevideo"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mockCredList(mux, "ak-transfer", "skey-transfer01")

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:             srv.URL + "/api/cloud/download",
		AccessKey:          ak,
		AccessKeySecret:    sk,
		AccessKeyID:        skid,
		TransferVolume:     "default",
		PullBackToSavePath: true,
		PollEvery:          10,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "out.mp4")}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if gotTransferVolume != "default" {
		t.Fatalf("transfer volume = %q, want default", gotTransferVolume)
	}
	// TransferURL 写入 obj.Extra
	obj.RLock()
	tu, _ := obj.Extra["transfer_url"].(string)
	obj.RUnlock()
	if !strings.Contains(tu, "task-t") {
		t.Fatalf("transfer_url = %q, want contains task-t", tu)
	}
	// PullBack 命中 SavePath
	if !downloadHit.Load() {
		t.Fatal("pullback download not hit")
	}
	got, err := os.ReadFile(obj.SavePath)
	if err != nil {
		t.Fatalf("read savepath: %v", err)
	}
	if string(got) != "fakevideo" {
		t.Fatalf("savepath content = %q, want fakevideo", got)
	}
}

// TestSproxyHybrid_TransferDefaultNoPullback 默认（TransferVolume 配置但 PullBack=false）
// 只转存不拉回（SavePath 无文件，TransferURL 已存）。
func TestSproxyHybrid_TransferDefaultNoPullback(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-n", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://default/cloud/task-n/movie.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-n", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-n", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://default/cloud/task-n/movie.mp4"})
	})
	mockCredList(mux, "ak-n", "skey-n000001")
	mux.HandleFunc("GET /download/chunk", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "should not download", http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       "ak-n",
		AccessKeySecret: "4444444444444444444444444444444444444444444444444444444444444444",
		AccessKeyID:     "skey-n000001",
		TransferVolume:  "default",
		PollEvery:       10,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "out.mp4")}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	obj.RLock()
	tu, _ := obj.Extra["transfer_url"].(string)
	obj.RUnlock()
	if !strings.Contains(tu, "task-n") {
		t.Fatalf("transfer_url = %q, want contains task-n", tu)
	}
	if _, err := os.Stat(obj.SavePath); !os.IsNotExist(err) {
		t.Fatalf("savepath should not exist (no pullback), stat err=%v", err)
	}
}

// mockCredList 在 mux 上挂 GET /api/credentials/<ak>/sk：返回含当前 skeyID 的 SK 列表
// （30d 过期，供 verifyOnStart 预热 expireAt）。
func mockCredList(mux *http.ServeMux, ak, skid string) {
	mux.HandleFunc("GET /api/credentials/"+ak+"/sk", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"ak": ak, "total": 1, "admin": false,
			"sk": []map[string]any{{
				"sk_id": skid, "created": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
				"expires": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
				"status":  "alive",
			}},
		})
	})
}

// TestSproxyHybrid_VerifyOnStartFail 验证：启动验证失败（ListAccessKeys 401）→ verified=false
// → Download 显式拒绝（用户要求：确认有效才能启动任务）。
func TestSproxyHybrid_VerifyOnStartFail(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/credentials/ak-bad/sk", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad sig", http.StatusUnauthorized)
	})
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-x", "status": "completed", "filename": "movie.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-x", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-x", "status": "completed", "filename": "movie.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       "ak-bad",
		AccessKeySecret: "5555555555555555555555555555555555555555555555555555555555555555",
		AccessKeyID:     "skey-bad000001",
	})
	if d.verified {
		t.Fatal("verified should be false after 401 verify-on-start")
	}
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "out.mp4")}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("Download should fail when verify-on-start failed")
	}
}

// TestSproxyHybrid_VerifyOnStartPrimesExpiry 验证：启动验证成功 → expireAt 预热（30d），
// 进入提前 24h 窗口时 ensureRotatedBeforeSubmit 真正触发轮换。
func TestSproxyHybrid_VerifyOnStartPrimesExpiry(t *testing.T) {
	t.Parallel()
	ak := "ak-prime"
	sk := "6666666666666666666666666666666666666666666666666666666666666666"
	skid := "skey-prime0001"
	newSK := "9999999999999999999999999999999999999999999999999999999999999999"
	var renewCount atomic.Int64
	// 信封：旧 SK 包裹新 SK（RenewAccessKey 解封需要真 wrapped_secret）
	skBytes := mustDecodeHex(t, sk)
	wk, err := accesskey.DeriveWrapKey(skBytes, ak, client.CredentialWrapContext(ak))
	if err != nil {
		t.Fatal(err)
	}
	env, err := accesskey.EncryptSecret(ak, mustDecodeHex(t, newSK), wk)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/credentials/ak-prime/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		writeJSONResp(w, map[string]any{"ak": ak, "sk_id": skid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			}})
	})
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-p", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	if !d.verified {
		t.Fatal("verify-on-start should succeed")
	}
	// 预热后 expireAt ≈ 30d 后；模拟时钟推进到到期前 23h
	d.expireAt = time.Now().Add(23 * time.Hour)
	d.lastRotate = time.Time{}
	if err := d.ensureRotatedBeforeSubmit(); err != nil {
		t.Fatalf("ensureRotated: %v", err)
	}
	if renewCount.Load() != 1 {
		t.Fatalf("renew called = %d, want 1 (within 24h window after prime)", renewCount.Load())
	}
}

// TestSproxyHybrid_Poll500DoesNotRotate 验证：poll 遇 500 不触发轮换（评审 P1-A/P1-2：
// 仅 401 轮换；5xx/网络错误只计数短路，不风暴）。
func TestSproxyHybrid_Poll500DoesNotRotate(t *testing.T) {
	t.Parallel()
	ak := "ak-500"
	sk := "7777777777777777777777777777777777777777777777777777777777777777"
	skid := "skey-50000001"
	var renewCount atomic.Int64
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/credentials/ak-500/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		writeJSONResp(w, map[string]any{"ak": ak, "sk_id": skid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339), "wrapped_secret": nil})
	})
	// 任务详情恒 500
	mux.HandleFunc("GET /api/cloud/tasks/task-500", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
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
	start := time.Now()
	_, err := d.pollWithRotateResult(context.Background(), "task-500")
	if err == nil {
		t.Fatal("poll should fail on 500")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("poll did not short-circuit, took %v", time.Since(start))
	}
	if renewCount.Load() != 0 {
		t.Fatalf("renew called = %d, want 0 (500 should NOT rotate)", renewCount.Load())
	}
}

// TestSproxyHybrid_Submit401LimitedRetry 验证 submit 401 有限重试（评审 P1-1：无界递归
// → DoS）：renew 后仍 401 → 最多重试 2 次后返回错误，不无限递归。
func TestSproxyHybrid_Submit401LimitedRetry(t *testing.T) {
	t.Parallel()
	ak := "ak-sub401"
	sk := "8888888888888888888888888888888888888888888888888888888888888888"
	skid := "skey-sub40101"
	newSK := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var renewCount atomic.Int64
	// 真信封：RenewAccessKey 解封成功 → 401 后继续重试
	skBytes := mustDecodeHex(t, sk)
	wk, err := accesskey.DeriveWrapKey(skBytes, ak, client.CredentialWrapContext(ak))
	if err != nil {
		t.Fatal(err)
	}
	env, err := accesskey.EncryptSecret(ak, mustDecodeHex(t, newSK), wk)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/credentials/ak-sub401/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		writeJSONResp(w, map[string]any{"ak": ak, "sk_id": skid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			}})
	})
	// 提交恒 401
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad creds", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	_, sErr := d.submitSig(context.Background(), "https://mypikpak.com/s/abc", "out.mp4", nil)
	if sErr == nil {
		t.Fatal("submit should fail after 401 retries exhausted")
	}
	// renew 最多 maxSubmitRetry 次（2），不无限递归
	if renewCount.Load() > 2 {
		t.Fatalf("renew called = %d, want <=2 (limited retry)", renewCount.Load())
	}
}
