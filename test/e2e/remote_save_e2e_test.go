// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e

// 真实 sproxy E2E：独立端口启动 download-manager（in-process），不重启 sproxy，
// 使用唯一测试目录，不影响其它上传任务。
//
// 运行示例（凭据来自环境，不写死）：
//
//	SPROXY_E2E_URL=https://127.0.0.1:18084 \
//	SPROXY_E2E_AK=... SPROXY_E2E_SK=... SPROXY_E2E_SKID=... \
//	go test -tags=e2e -run TestRemoteSaveRealE2E -v ./test/e2e
import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/api"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/remotesave"
	_ "github.com/cocomhub/download-manager/task/urllist"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

func TestRemoteSaveRealE2E(t *testing.T) {
	base := strings.TrimRight(os.Getenv("SPROXY_E2E_URL"), "/")
	ak, sk, skid := os.Getenv("SPROXY_E2E_AK"), os.Getenv("SPROXY_E2E_SK"), os.Getenv("SPROXY_E2E_SKID")
	if base == "" || ak == "" || sk == "" || skid == "" {
		t.Skip("需要 SPROXY_E2E_URL/SPROXY_E2E_AK/SPROXY_E2E_SK/SPROXY_E2E_SKID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	run := fmt.Sprintf("e2e/remote-save-%d", time.Now().UnixNano())
	content := []byte("remote-save-e2e-" + time.Now().Format(time.RFC3339Nano))
	sha := sha256.Sum256(content)
	shaHex := hex.EncodeToString(sha[:])

	fc := sproxyclient.NewFileClient(base,
		sproxyclient.WithVolume("main"),
		sproxyclient.WithAccessKey(ak, sk),
		sproxyclient.WithAccessKeyID(skid),
		sproxyclient.WithInsecureTLS(),
		sproxyclient.WithTimeout(30*time.Second),
	)
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		_, _ = w.Write(content)
	}))
	defer fileSrv.Close()
	objKey := remotesave.ObjectKey(&model.DownloadObject{URL: fileSrv.URL + "/ABC-001.mp4"})
	remote := run + "/" + objKey + "/ABC-001.mp4"
	// 冲突前置：远端先放不同内容。
	if _, err := fc.Upload(ctx, writeTemp(t, []byte("conflict-existing")), remote); err != nil {
		t.Fatalf("pre-upload conflict: %v", err)
	}
	t.Cleanup(func() {
		_ = fc.Delete(context.Background(), remote, "")
		_ = fc.Delete(context.Background(), path.Dir(remote), "")
	})

	dir := t.TempDir()
	config.SetConfigFilePath(filepath.Join(dir, "config.yaml"))
	for _, d := range []string{"downloads", "downloads/e2e", "logs"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		Server:   config.Server{HTTPPort: 0, WorkDir: dir, DownloadRootDir: filepath.Join(dir, "downloads")},
		Runtime:  config.Runtime{Mode: config.RunModeFull},
		TaskScan: config.TaskScan{Interval: 1},
		Downloader: config.Downloader{
			GlobalConcurrent: 2, MaxRetries: 3,
			Filesystem: config.DcFilesystem{RootDir: filepath.Join(dir, "downloads"), LogDir: filepath.Join(dir, "logs")},
		},
		Sproxy: config.Sproxy{
			APIURL: base, AccessKey: ak, AccessKeySecret: sk, AccessKeyID: skid,
			InsecureTLS: true, PlayBaseURL: base, PlayTTL: time.Hour,
		},
		RemoteSave: config.RemoteSaveSettings{
			Concurrency: 1, ReconcileInterval: time.Hour, DeepReconcileInterval: 24 * time.Hour,
			SettleSeconds: 1, MaxRetries: 2, MaxFilesPerObject: 100,
		},
		Tasks: []config.Task{{
			ID: "e2e-remote-save", Type: "url_list",
			SaveDir: filepath.Join(dir, "downloads", "e2e"),
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"urls": []string{fileSrv.URL + "/ABC-001.mp4"},
				"remote_save": map[string]any{
					"enabled": true, "volume": "main", "dir": run,
					"object_dirs": true, "mirror_delete": true,
				},
			},
		}},
	}
	cfg.Runtime.Download.Enabled = true
	cfg.Runtime.Scheduler.Enabled = true
	cfg.ValidateAndClamp()
	config.SetServerConfig(cfg.Server)
	mgr := manager.NewManager(cfg)
	go mgr.Start()
	defer func() {
		stop, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		mgr.Stop(stop)
	}()
	srv := httptest.NewServer(api.NewServer(mgr).Router())
	defer srv.Close()

	// 等待对象 completed。
	var obj model.DownloadObject
	waitFor(t, 60*time.Second, func() bool {
		body := getJSON(t, srv.URL+"/api/tasks/e2e-remote-save")
		objs, _ := body["objects"].([]any)
		if len(objs) == 0 {
			return false
		}
		m, _ := objs[0].(map[string]any)
		if m["status"] != model.StatusCompleted {
			return false
		}
		b, _ := json.Marshal(m)
		_ = json.Unmarshal(b, &obj)
		return true
	}, "object completed")

	// 触发同步 → 冲突。
	postJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save", map[string]any{"force": true})
	waitFor(t, 30*time.Second, func() bool {
		body := getJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save/conflicts")
		return int(body["total"].(float64)) > 0
	}, "conflict detected")
	conflicts := getJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save/conflicts")
	list := conflicts["conflicts"].([]any)
	c0 := list[0].(map[string]any)
	if c0["reason"] != "checksum_mismatch" {
		t.Fatalf("conflict reason=%v", c0["reason"])
	}
	// 人工覆盖。
	resolveResp := postJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save/conflicts/resolve", map[string]any{
		"object_url": obj.URL, "path": c0["path"], "action": "overwrite",
	})
	t.Logf("resolve response: %#v", resolveResp)
	waitFor(t, 30*time.Second, func() bool {
		st := getJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save")
		stt, _ := st["statuses"].(map[string]any)
		return stt[model.RemoteSaveStatusSynced] != nil
	}, "synced after overwrite")

	fi, err := fc.Stat(ctx, remote)
	if err != nil {
		t.Fatalf("stat remote: %v", err)
	}
	if !strings.EqualFold(fi.Checksum, shaHex) {
		t.Fatalf("remote sha=%s want %s", fi.Checksum, shaHex)
	}
	mi, err := fc.GetFileMetaInfo(ctx, remote)
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if mi.MTime == "" {
		t.Fatal("meta mtime 为空（应保存原始 mtime）")
	}
	if mi.Extra["x_dm_object"] == nil {
		t.Fatalf("meta 缺少 x_dm_object: %#v", mi.Extra)
	}
	xf, _ := mi.Extra["x_dm_file"].(map[string]any)
	if xf == nil || !strings.EqualFold(fmt.Sprint(xf["sha256"]), shaHex) {
		t.Fatalf("meta x_dm_file.sha256=%v", xf)
	}

	// 播放 URL 真实可读（Range 由 sproxy 处理）。
	playResp := getJSON(t, srv.URL+"/api/tasks/e2e-remote-save/object/play_url?url="+urlQuery(obj.URL))
	playURL := fmt.Sprint(playResp["url"])
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec
	resp, err := client.Get(playURL)
	if err != nil {
		t.Fatalf("play url: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("play status=%d content=%q want %q", resp.StatusCode, body, content)
	}

	// 镜像删除：删除对象后触发对账，远端应被清理。
	postJSON(t, srv.URL+"/api/tasks/e2e-remote-save/object/delete_batch", map[string]any{"urls": []string{obj.URL}})
	postJSON(t, srv.URL+"/api/tasks/e2e-remote-save/remote_save", map[string]any{"force": true})
	waitFor(t, 30*time.Second, func() bool {
		_, err := fc.Stat(ctx, remote)
		return err != nil
	}, "mirror delete")

}

func writeTemp(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitFor(t *testing.T, d time.Duration, fn func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out
}

func postJSON(t *testing.T, url string, body map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 400 {
		t.Logf("POST %s status=%d body=%v", url, resp.StatusCode, out)
	}
	return out
}

func urlQuery(s string) string {
	return neturl.QueryEscape(s)
}
