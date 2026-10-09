// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"

	"github.com/cocomhub/sproxy/pkg/accesskey"
	"github.com/cocomhub/sproxy/pkg/client"
)

// TestSproxyCloud_TransferPath 验证转存目标子路径/重命名透传（transfer.path，
// 支持形如 xxx/xxxx.mp4 的卷内子目录）。
func TestSproxyCloud_TransferPath(t *testing.T) {
	t.Parallel()
	ak := "ak-tp"
	sk := "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
	skid := "skey-tp00000001"
	var gotPath, gotVol string
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if tr, ok := body["transfer"].(map[string]any); ok {
			gotVol, _ = tr["volume"].(string)
			gotPath, _ = tr["path"].(string)
		}
		writeJSONResp(w, map[string]any{"id": "task-tp", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://vol/xxx/xxxx.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-tp", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-tp", "status": "completed", "filename": "movie.mp4",
			"transfer_url": "sproxy://vol/xxx/xxxx.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		TransferVolume:  "vol",
		TransferPath:    "xxx/xxxx.mp4",
		PollEvery:       10,
		CloudOnly:       true, // 只验 transfer 透传，不下载到本地
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "out.mp4")}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if gotVol != "vol" || gotPath != "xxx/xxxx.mp4" {
		t.Fatalf("transfer = {volume:%q path:%q}, want {vol xxx/xxxx.mp4}", gotVol, gotPath)
	}
	obj.RLock()
	tu, _ := obj.Extra["transfer_url"].(string)
	tid, _ := obj.Extra["cloud_task_id"].(string)
	obj.RUnlock()
	if !strings.Contains(tu, "xxx/xxxx.mp4") {
		t.Fatalf("transfer_url = %q", tu)
	}
	if tid != "task-tp" {
		t.Fatalf("cloud_task_id = %q, want task-tp", tid)
	}
}

// TestSproxyCloud_DefaultNoArtifactRecordsCoords 验证：默认（不转存不拉回）也记录
// cloud 桶坐标，供上层取用（对抗性评审 P1-2：避免静默无产物且无线索）。
func TestSproxyCloud_DefaultNoArtifactRecordsCoords(t *testing.T) {
	t.Parallel()
	ak := "ak-def"
	sk := "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
	skid := "skey-def0000001"
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-def", "status": "running", "filename": "movie.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-def", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-def", "status": "completed", "filename": "movie.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       10,
		CloudOnly:       true, // 只验坐标落盘，不下载到本地
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	obj.RLock()
	tid, _ := obj.Extra["cloud_task_id"].(string)
	fn, _ := obj.Extra["cloud_task_filename"].(string)
	obj.RUnlock()
	if tid != "task-def" || fn != "movie.mp4" {
		t.Fatalf("coords = {%q %q}, want {task-def movie.mp4}", tid, fn)
	}
}

// TestSproxyCloud_PullBackWithoutSigFails 验证 Bearer 模式配 pull_back 时显式报错
// （不静默成功——对抗性评审 P1-1）。
func TestSproxyCloud_PullBackWithoutSigFails(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-pb", "status": "completed", "filename": "movie.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-pb", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-pb", "status": "completed", "filename": "movie.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		APIToken:  "bearer",
		PollEvery: 10,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "o.mp4")}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("Download should fail: pullback requires SproxySig")
	}
}

// TestSproxyCloud_BearerTransfers 验证 Bearer 路径也透传 transfer（不静默丢弃）。
func TestSproxyCloud_BearerTransfers(t *testing.T) {
	t.Parallel()
	var gotVol string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if tr, ok := body["transfer"].(map[string]any); ok {
			gotVol, _ = tr["volume"].(string)
		}
		writeJSONResp(w, map[string]any{"id": "task-bt", "status": "completed", "filename": "m.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-bt", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-bt", "status": "completed", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:         srv.URL + "/api/cloud/download",
		APIToken:       "bearer",
		TransferVolume: "vol2",
		PollEvery:      10,
		CloudOnly:      true, // 只验 transfer 透传，不下载到本地
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if gotVol != "vol2" {
		t.Fatalf("bearer transfer volume = %q, want vol2", gotVol)
	}
}

// TestSproxyCloud_PollPersistent401RotationCapped 验证持久 401 下轮换次数有上限
// （防 renew 风暴——对抗性评审 P1-3）。
func TestSproxyCloud_PollPersistent401RotationCapped(t *testing.T) {
	t.Parallel()
	ak := "ak-cap"
	sk := "cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333cccc3333"
	skid := "skey-cap0000001"
	newSK := "dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444"
	var renewCount atomic.Int64
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
	mux.HandleFunc("POST /api/credentials/ak-cap/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		writeJSONResp(w, map[string]any{"ak": ak, "sk_id": skid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			}})
	})
	// 任务详情恒 401（即使轮换后仍 401）
	mux.HandleFunc("GET /api/cloud/tasks/task-cap", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad creds", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       5,
		Timeout:         2 * time.Second,
	})
	start := time.Now()
	_, err = d.pollWithRotateResult(context.Background(), "task-cap")
	if err == nil {
		t.Fatal("poll should fail on persistent 401")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("poll took too long: %v", time.Since(start))
	}
	if renewCount.Load() > 3 {
		t.Fatalf("renew called = %d, want capped (<=3) to avoid credential storm", renewCount.Load())
	}
}

// TestSproxyCloud_RotateSingleFlight 验证并发轮换 single-flight：并发调用只真正
// renew 一次（防并发数据竞争/凭据风暴——对抗性评审 P1-4）。
func TestSproxyCloud_RotateSingleFlight(t *testing.T) {
	t.Parallel()
	ak := "ak-sf"
	sk := "eeee5555eeee5555eeee5555eeee5555eeee5555eeee5555eeee5555eeee5555"
	skid := "skey-sf00000001"
	newSK := "ffff6666ffff6666ffff6666ffff6666ffff6666ffff6666ffff6666ffff6666"
	var renewCount atomic.Int64
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
	mux.HandleFunc("POST /api/credentials/ak-sf/renew", func(w http.ResponseWriter, r *http.Request) {
		renewCount.Add(1)
		time.Sleep(60 * time.Millisecond) // 拉长窗口以确保并发重叠
		writeJSONResp(w, map[string]any{"ak": ak, "sk_id": skid, "kind": "symmetric", "wrap_key_ak": ak,
			"expires_at": time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"wrapped_secret": map[string]any{
				"kind": env.Kind, "wrap_key_id": env.WrapKeyID, "nonce": env.Nonce, "ciphertext": env.Cipher,
			}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_ = d.rotateOnce()
		})
	}
	wg.Wait()
	if n := renewCount.Load(); n != 1 {
		t.Fatalf("renew called = %d, want 1 (single-flight)", n)
	}
}
