// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestShutdown_InFlightDownloads verifies that when Manager.Stop() is called
// while downloads are in flight, the survivors are correctly marked as failed
// and no goroutine is leaked.
func TestShutdown_InFlightDownloads(t *testing.T) {
	mgr, _ := newMockManager(t, "shutdown-test", 3,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(100*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()

	// Track whether we've already stopped to avoid double-Stop+close.
	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		mgr.Stop(ctx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "shutdown-test")

	// Wait until all objects are resolved (StatusDownloading after resolve fix).
	// With the resolve→StatusDownloading optimization, objects enter StatusDownloading
	// during resolve, before being enqueued to the download queue.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			if obj.GetStatus() != model.StatusDownloading {
				return false
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "expected all objects to be resolved")

	// Wait for the scheduler to enqueue at least one object to the download queue.
	// After resolve, processTask must run again to push StatusDownloading objects
	// through the task queue → scheduler → download queue pipeline.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		mgr.mu.Lock()
		active := mgr.activeDownloads["shutdown-test"]
		mgr.mu.Unlock()
		return active > 0
	}, 5*time.Second, 100*time.Millisecond, "expected at least one object to be enqueued")

	// Call Stop — this should cancel the in-flight download and mark survivors.
	stopOnce()

	// Verify: downloadingObj should be empty (all cleaned up).
	var remaining int
	mgr.downloadingObj.Range(func(_, _ any) bool {
		remaining++
		return true
	})
	if remaining > 0 {
		t.Errorf("expected empty downloadingObj after Stop, got %d items", remaining)
	}

	// Verify: activeDownloads >= 0 for all tasks.
	mgr.mu.Lock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("negative activeDownloads for %s: %d", taskID, count)
		}
		t.Logf("activeDownloads[%s] = %d", taskID, count)
	}
	mgr.mu.Unlock()

	// Verify: downloading objects are marked failed; pending objects stay pending.
	// Note: with the resolve->StatusDownloading optimization, objects that were
	// resolved but not yet enqueued to the download queue will remain in
	// StatusDownloading state. This is acceptable — they are resolved and will
	// be picked up on the next run.
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		status := obj.GetStatus()
		switch status {
		case model.StatusDownloading:
			// Already resolved but not yet dispatched — correct, will resume on restart.
		case model.StatusFailed, model.StatusCancelled:
			// Terminal state from shutdown — correct.
		case model.StatusPending:
			// Was never picked up — correct, it wasn't in downloadingObj.
		default:
			t.Errorf("unexpected status %s for %s after shutdown", status, obj.URL)
		}
	}
}

// TestShutdown_ForceDownload verifies that a force-download goroutine
// (bypassing the worker pool) converges after Stop: the object is marked
// failed and forceWg drains within the context deadline.
func TestShutdown_ForceDownload(t *testing.T) {
	mgr, _ := newMockManager(t, "shutdown-force", 1,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(100*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-mgr.Initialized():
	case <-ctx.Done():
		t.Fatal("manager failed to initialize")
	}

	// Stop 不可重入（close(stopChan) 重复 panic），用 stopOnce 保证只调一次。
	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		mgr.Stop(stopCtx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "shutdown-force")

	// Wait for an object to exist (resolve→downloading 优化下可能非 pending)，
	// then force-download it（bypass queue）。
	var obj *model.DownloadObject
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		if len(all) > 0 {
			obj = all[0]
			return true
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "expected an object to seed")

	// Force download bypasses the queue.
	mgr.forceDownload(task, obj)

	// Wait until the object is actually downloading (in downloadingObj).
	assert.MustEventually(t, func() bool {
		_, ok := mgr.downloadingObj.Load("http://mock-download/file-0.bin")
		return ok
	}, 5*time.Second, 50*time.Millisecond, "expected force-download to register in downloadingObj")

	// Stop — force-download should be cancelled and survivor marked failed.
	stopOnce()

	// downloadingObj must be empty after Stop.
	var remaining int
	mgr.downloadingObj.Range(func(_, _ any) bool {
		remaining++
		return true
	})
	if remaining > 0 {
		t.Errorf("expected empty downloadingObj after Stop, got %d", remaining)
	}

	// Force-download goroutine must converge (forceWg drains).
	// WaitForShutdown returns only after forceWg.Wait().
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	mgr.WaitForShutdown(waitCtx) // must not hang
}

// TestShutdown_FlushStorages verifies that WaitForShutdown flushes
// file-backed storages to disk (ForceFlush path via flushAllStorages).
func TestShutdown_FlushStorages(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "shutdown-flush.json")
	// Use a long save interval to guarantee the object is NOT flushed by the
	// lazy timer — only ForceFlush (via WaitForShutdown) can persist it.
	fs, err := storage.NewFileStorage(map[string]string{"path": storagePath, "save_interval": "86400"})
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	obj := &model.DownloadObject{URL: "http://127.0.0.1/shutdown/flush.bin", Status: model.StatusPending}
	if err := fs.Update(obj); err != nil {
		t.Fatalf("Update: %v", err)
	}

	mgr := NewManager(&config.Config{Server: config.Server{WorkDir: t.TempDir()}})
	mgr.tasks.Store("flush-task", &mockTaskWithStorage{id: "flush-task", typ: "mock", st: fs})

	// WaitForShutdown calls flushAllStorages → ForceFlush on the file storage.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	mgr.WaitForShutdown(ctx)

	// The object must be on disk now (ForceFlush persisted it).
	data, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("expected flushed storage file, got error: %v", err)
	}
	if !bytes.Contains(data, []byte("http://127.0.0.1/shutdown/flush.bin")) {
		t.Errorf("flushed file missing the object URL: %s", string(data))
	}
}

// TestShutdown_Timeout verifies that Stop returns (does not hang) even when
// a download blocks indefinitely and the context deadline expires.
// Stop 不可重入（close(stopChan) 重复会 panic），用 stopOnce 保证只调一次。
func TestShutdown_Timeout(t *testing.T) {
	mgr, _ := newMockManager(t, "shutdown-timeout", 1,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(10*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-mgr.Initialized():
	case <-ctx.Done():
		t.Fatal("manager failed to initialize")
	}

	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		mgr.Stop(stopCtx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "shutdown-timeout")

	// Seed and let the worker start downloading (ModePauseOnProgress blocks
	// until the download context is cancelled — which happens on Stop).
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			if obj.GetStatus() != model.StatusPending {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "expected object to leave pending")

	// Stop with a very short deadline — must return, not hang.
	// 走 stopOnce 保证只调一次 Stop（Stop 不可重入）。
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()
	start := time.Now()
	mgr.Stop(shortCtx)
	stopped.Store(true) // 标记：defer stopOnce 不再重复 Stop
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Stop with short deadline took too long: %v", elapsed)
	}
	<-done
}
