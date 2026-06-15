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

// --- objectTracker 追踪单个 DownloadObject 的所有下载项完成状态 ---

type objectTracker struct {
	mu    sync.Mutex
	total int // 总下载项数（小对象数）
	done  int // 已完成数
	errs  []error
	ch    chan struct{} // 全部完成时关闭
}

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

// WaitAll 等待所有小对象完成，返回遇到的错误列表。
// timeout 为最大等待时间，超时时返回已收集的错误。
func (t *objectTracker) WaitAll(timeout time.Duration) []error {
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

// --- Manager 小对象调度 ---

// StartSmallObjectWorkers 启动小对象下载 worker 协程池。
func (m *Manager) StartSmallObjectWorkers(n int) {
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

// StopSmallObjectWorkers 停止小对象下载 worker 协程池。
func (m *Manager) StopSmallObjectWorkers() {
	if m.soStop == nil {
		return
	}
	close(m.soStop)
	m.soWg.Wait()
}

// enqueueSmallObjects 检查 task 是否实现了 SmallObjectCap，有则入队。
func (m *Manager) enqueueSmallObjects(t core.Task, obj *model.DownloadObject) *objectTracker {
	soc, ok := t.(core.SmallObjectCap)
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
			tracker.MarkDone(nil) // 不阻塞主流程
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

	// 小对象重试：最多 3 次，指数退避
	const maxAttempts = 3
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

	// 下载成功后将路径写回 parentObj.Extra，供前端读取
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
