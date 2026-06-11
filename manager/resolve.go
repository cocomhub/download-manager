// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/model"
)

type resolveRequest struct {
	taskID string
	obj    *model.DownloadObject
}

// StartResolveWorkers 启动 resolve worker 协程池。
// 只在 Manager.Start() 中调用一次。
func (m *Manager) StartResolveWorkers(n int) {
	if n <= 0 {
		n = 3
	}
	m.resolveQueue = make(chan resolveRequest, 128)
	m.resolveStop = make(chan struct{})
	for i := 0; i < n; i++ {
		m.resolveWg.Add(1)
		go m.resolveWorker(i)
	}
	slog.Info("Resolve workers started", "count", n)
}

// StopResolveWorkers 停止 resolve worker 协程池。
func (m *Manager) StopResolveWorkers() {
	if m.resolveStop == nil {
		return
	}
	close(m.resolveStop)
	m.resolveWg.Wait()
}

// enqueueResolve 将需要 resolve 的对象放入队列。
func (m *Manager) enqueueResolve(taskID string, obj *model.DownloadObject) {
	select {
	case m.resolveQueue <- resolveRequest{taskID: taskID, obj: obj}:
	default:
		slog.Warn("Resolve queue full, dropping", "task_id", taskID, "url", obj.URL)
	}
}

func (m *Manager) resolveWorker(id int) {
	defer m.resolveWg.Done()
	slog.Debug("Resolve worker started", "id", id)
	for {
		select {
		case <-m.resolveStop:
			return
		case req := <-m.resolveQueue:
			m.processResolve(req)
		}
	}
}

func (m *Manager) processResolve(req resolveRequest) {
	t, ok := m.getTask(req.taskID)
	if !ok {
		slog.Warn("Resolve: task not found", "task_id", req.taskID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := t.ResolveObject(ctx, req.obj); err != nil {
		slog.Error("Resolve failed", "task_id", req.taskID, "url", req.obj.URL, "error", err)
		_ = t.UpdateStatus(req.obj, model.StatusFailed, err)
		m.resolveCache.Invalidate(req.obj.URL)
		return
	}

	m.resolveCache.MarkResolved(req.obj.URL)
	// resolve 成功后设回 Pending，下一轮 processTask 会将其加入 candidates
	_ = t.UpdateStatus(req.obj, model.StatusPending, nil)
	slog.Debug("Resolve succeeded", "task_id", req.taskID, "url", req.obj.URL)
}
