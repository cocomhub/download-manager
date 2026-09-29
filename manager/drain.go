// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// StartDrain 触发排空退出：完成当前在途下载后退出进程。
//
// 语义（用户确认 2026-09-28）：
//   - 仅等「真正正在 Download()」的对象（inflight 跟踪）跑完；
//   - 已入队未开始的对象丢弃，重置为 pending（下次启动可续跑）；
//   - 主任务附带的小对象（封面/预览）随主任务跑完（download() 内 WaitAll 保证）；
//   - 无超时上限。
//
// 幂等：多次调用只触发一次。返回 false 表示已处于 drain/停止中。
func (m *Manager) StartDrain() bool {
	if !m.drainMode.CompareAndSwap(false, true) {
		return false
	}
	m.drainOnceStarted.Store(true)
	slog.Info("Drain-to-exit triggered: finishing in-flight downloads, no new tasks")
	m.reclaimQueuedOnDrain()
	m.signalDrain()
	// 触发时已无在途下载（且排队项已回收为 pending）→ 立即完成排空，
	// 避免 WaitForDrain 永久阻塞（drainDone 唯一关闭点是 download() defer）。
	if !m.hasInflight() {
		m.notifyDrainDone()
	}
	return true
}

// reclaimQueuedOnDrain 把已登记 downloadingObj 但尚未真正开始下载（不在 inflight）
// 的排队项回收回 pending 状态并释放下载槽位，供下次启动续跑。
// 正在下载（inflight）的对象不受影响，由 worker 照常完成。
func (m *Manager) reclaimQueuedOnDrain() {
	m.downloadingObj.Range(func(key, value any) bool {
		url := key.(string)
		obj := value.(*model.DownloadObject)
		// 在途（worker 正在 Download）→ 跳过，等它完成。
		if _, inflight := m.inflight.Load(url); inflight {
			return true
		}
		// 排队未取：移除下载登记，状态重置回 pending。
		m.downloadingObj.Delete(url)
		m.inflight.Delete(url)
		m.mu.Lock()
		if m.activeDownloads[obj.TaskID] > 0 {
			m.activeDownloads[obj.TaskID]--
		}
		m.mu.Unlock()
		if t, ok := m.getTask(obj.TaskID); ok {
			// 重置 pending（即使已是 completed，也只在非终态时改）。
			switch obj.GetStatus() {
			case model.StatusCompleted, model.StatusFailed, model.StatusFailedPermanent, model.StatusCancelled:
			default:
				_ = t.UpdateStatus(obj, model.StatusPending, nil)
			}
		}
		return true
	})
}

// DrainRequested 返回是否已触发排空退出。
func (m *Manager) DrainRequested() bool {
	return m.drainMode.Load()
}

// DrainStartedOnce 报告排空是否由本次调用真正启动（幂等判据）。
func (m *Manager) DrainStartedOnce() bool {
	return m.drainOnceStarted.Load()
}

// DrainDone 返回排空完成通知 channel：在途下载全部跑完后关闭。
func (m *Manager) DrainDone() <-chan struct{} {
	return m.drainDone
}

// signalDrain 唤醒调度器/worker，使其停止取新任务并退出取循环。
func (m *Manager) signalDrain() {
	select {
	case m.schedulerSignal <- struct{}{}:
	default:
	}
}

// WaitForDrain 等待在途下载全部完成（无超时）。
// 返回时所有 inflight 对象已结束（成功/失败），进程可安全退出。
func (m *Manager) WaitForDrain(ctx context.Context) {
	slog.Info("Waiting for in-flight downloads to finish")
	<-m.drainDone
	select {
	case <-ctx.Done():
	default:
	}
}

// notifyDrainDone 由最后一个 inflight 对象结束时调用，关闭 drainDone。
func (m *Manager) notifyDrainDone() {
	m.drainOnce.Do(func() {
		close(m.drainDone)
		slog.Info("In-flight downloads finished, drain complete")
	})
}

var _ = logutil.LogKeyTaskID
