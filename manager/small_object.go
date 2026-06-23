// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
)

// --- objectTracker 杩借釜鍗曚釜 DownloadObject 鐨勬墍鏈変笅杞介」瀹屾垚鐘舵€?---

type objectTracker struct {
	mu    sync.Mutex
	total int // 鎬讳笅杞介」鏁帮紙灏忓璞℃暟锛?	done  int // 宸插畬鎴愭暟
	errs  []error
	ch    chan struct{} // 鍏ㄩ儴瀹屾垚鏃跺叧闂?}

func newObjectTracker(total int) *objectTracker {
	return &objectTracker{total: total, ch: make(chan struct{})}
}

func (t *objectTracker) MarkDone(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done++
	if err != nil {
		t.errs = append(t.errs, err)
	}
	if t.done >= t.total {
		close(t.ch)
	}
}

// WaitAll 绛夊緟鎵€鏈夊皬瀵硅薄瀹屾垚锛岃繑鍥為亣鍒扮殑閿欒鍒楄〃銆?// timeout 涓烘渶澶х瓑寰呮椂闂达紝瓒呮椂鏃惰繑鍥炲凡鏀堕泦鐨勯敊璇€?func (t *objectTracker) WaitAll(timeout time.Duration) []error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-t.ch:
	case <-timer.C:
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.errs
}

// --- smallObjectRequest ---

type smallObjectRequest struct {
	taskID    string
	parentObj *model.DownloadObject
	info      core.SmallObjectInfo
	tracker   *objectTracker
}

// --- Manager 灏忓璞¤皟搴?---

// StartSmallObjectWorkers 鍚姩灏忓璞′笅杞?worker 鍗忕▼姹犮€?func (m *Manager) StartSmallObjectWorkers(n int) {
	if n <= 0 {
		n = 2
	}
	m.soQueue = make(chan smallObjectRequest, 128)
	m.soStop = make(chan struct{})
	for i := 0; i < n; i++ {
		m.soWg.Add(1)
		go m.soWorker(i)
	}
	slog.Info("Small-object workers started", "count", n)
}

// StopSmallObjectWorkers 鍋滄灏忓璞′笅杞?worker 鍗忕▼姹犮€?func (m *Manager) StopSmallObjectWorkers() {
	if m.soStop == nil {
		return
	}
	close(m.soStop)
	m.soWg.Wait()
}

// enqueueSmallObjects 妫€鏌?task 鏄惁瀹炵幇浜?SmallObjectCap锛屾湁鍒欏叆闃熴€?func (m *Manager) enqueueSmallObjects(t core.Task, obj *model.DownloadObject) *objectTracker {
	soc, ok := t.(core.SmallObjectProvider)
	if !ok {
		return nil
	}

	items := soc.SmallObjects(obj)
	if len(items) == 0 {
		return nil
	}

	tracker := newObjectTracker(len(items))
	m.soTracker.Store(obj.URL, tracker)

	for _, info := range items {
		select {
		case m.soQueue <- smallObjectRequest{
			taskID:    t.ID(),
			parentObj: obj,
			info:      info,
			tracker:   tracker,
		}:
		default:
			slog.Warn("Small-object queue full, dropping", "task_id", t.ID(), "url", obj.URL, "rel", info.Rel)
			tracker.MarkDone(nil) // 涓嶉樆濉炰富娴佺▼
		}
	}
	return tracker
}

func (m *Manager) soWorker(id int) {
	defer m.soWg.Done()
	slog.Debug("Small-object worker started", "id", id)
	for {
		select {
		case <-m.soStop:
			return
		case req := <-m.soQueue:
			m.processSO(req)
		}
	}
}

func (m *Manager) processSO(req smallObjectRequest) {
	slog.Debug("Downloading small object", "task_id", req.taskID,
		"parent_url", req.parentObj.URL, "url", req.info.URL, "rel", req.info.Rel)

	// 灏忓璞￠噸璇曪細鏈€澶?3 娆★紝鎸囨暟閫€閬?	const maxAttempts = 3
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		dlCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err = download.Get(dlCtx, req.info.URL, req.info.SavePath)
		cancel()
		if err == nil {
			break
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			break
		}
		if attempt < maxAttempts {
			backoff := time.Duration(attempt*5) * time.Second
			slog.Warn("Small-object download failed, retrying", "task_id", req.taskID,
				"url", req.info.URL, "rel", req.info.Rel, "attempt", attempt, "backoff", backoff, "error", err)
			time.Sleep(backoff)
		}
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		slog.Error("Small-object download failed after retries", "task_id", req.taskID,
			"url", req.info.URL, "rel", req.info.Rel, "error", err)
	}

	// 涓嬭浇鎴愬姛鍚庡皢璺緞鍐欏洖 parentObj.Extra锛屼緵鍓嶇璇诲彇
	if err == nil {
		switch rel := req.info.Rel; rel {
		case "cover", "thumb":
			req.parentObj.Extra["local_cover"] = req.info.SavePath
		case "preview":
			req.parentObj.Extra["local_preview"] = req.info.SavePath
		}
	}

	req.tracker.MarkDone(err)
}
