// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type resolveRequest struct {
	taskID string
	obj    *model.DownloadObject
}

// StartResolveWorkers 鍚姩 resolve worker 鍗忕▼姹犮€?// 鍙湪 Manager.Start() 涓皟鐢ㄤ竴娆°€?func (m *Manager) StartResolveWorkers(n int) {
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

// StopResolveWorkers 鍋滄 resolve worker 鍗忕▼姹犮€?func (m *Manager) StopResolveWorkers() {
	if m.resolveStop == nil {
		return
	}
	close(m.resolveStop)
	m.resolveWg.Wait()
}

// enqueueResolve 灏嗛渶瑕?resolve 鐨勫璞℃斁鍏ラ槦鍒椼€?func (m *Manager) enqueueResolve(taskID string, obj *model.DownloadObject) {
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

	// resolve 鎴愬姛鍚庤鍥?Pending锛屼笅涓€杞?processTask 浼氬皢鍏跺姞鍏?candidates銆?	//
	// 浣跨敤 SetStatusUnlessCancelled 鍘熷瓙鍦版鏌ュ璞℃槸鍚﹀凡琚?CancelObject() 鍙栨秷锛?	// 鍦?b.mu 淇濇姢涓嬪畬鎴愯-妫€鏌?鍐欙紝娑堥櫎 TOCTOU 绔炰簤绐楀彛銆?	// 濡傛灉瀵硅薄宸茶鍙栨秷鎴栦换鍔′笉鏀寔姝ゅ畧鍗紝鍒欒烦杩囨洿鏂般€?	if guard, ok := t.(core.TaskStatusGuarder); ok {
		if !guard.SetStatusUnlessCancelled(req.obj, model.StatusPending, nil) {
			slog.Info("Resolve: object was cancelled, preserving cancelled status",
				"task_id", req.taskID, "url", req.obj.URL)
			m.resolveCache.Invalidate(req.obj.URL)
			return
		}
	} else {
		_ = t.UpdateStatus(req.obj, model.StatusPending, nil)
	}
	slog.Debug("Resolve succeeded", "task_id", req.taskID, "url", req.obj.URL)
}
