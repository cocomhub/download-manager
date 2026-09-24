// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
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

// smallObjectKey 返回小对象在途去重（soInflight）的 key。
// 用 taskID 加前缀，避免多个任务共享 SaveDir 时相对路径碰撞。
func smallObjectKey(taskID string, info core.SmallObjectInfo) string {
	if info.SavePath != "" {
		return taskID + "\x00" + info.SavePath
	}
	return taskID + "\x00" + info.URL
}

// fileExistsNonEmpty 判断本地文件是否存在且非空。
// 与 mxs ensureCoverLocal 的 Size()>0 启发式一致：非空即视为已下载，避免重复下载。
func fileExistsNonEmpty(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}

// finalizeSmallObject 将小对象的下载结果写回父对象媒体字段，并设置旧兼容字段。
// enqueue 时的「文件已存在」跳过 与 processSO 成功路径 共用，确保那些只在下载成功时写
// 媒体字段的任务类型（如 tktube preview）在文件已存在时也能把字段写回。
func finalizeSmallObject(parent *model.DownloadObject, info core.SmallObjectInfo) {
	if parent == nil {
		return
	}
	rel := info.Rel
	parent.SetMedia(rel, info.URL, info.SavePath)
	switch rel {
	case "cover", "thumb":
		parent.SetLocalCover(info.SavePath)
	case "preview":
		parent.SetLocalPreview(info.SavePath)
	}
}

// pendingSO 记录因队列满被丢弃、待后续补下载的小对象。
// 补下载只需把文件落地：媒体字段已由回填/构建写入，processSO 对 nil parentObj 的 finalize
// 为 no-op（SetMedia 等访问器对 nil receiver 安全）。
type pendingSO struct {
	taskID string
	info   core.SmallObjectInfo
}

// drainPendingSO 尽力把待补集合里的小对象重新入队（best-effort）。
// 队列仍满则停止本次排空（保留在 pending，下次再试）；文件已落地则直接清除。
// 由 enqueueSmallObjects 与 soWorker 调用，形成「drop 后自动补下载」的自愈机制。
func (m *Manager) drainPendingSO() {
	m.soPending.Range(func(key, value any) bool {
		p := value.(pendingSO)
		// 已落地则无需再补。
		if fileExistsNonEmpty(p.info.SavePath) {
			m.soPending.Delete(key)
			return true
		}
		// 与 enqueueSmallObjects 一致：入队前登记在途，避免并发 drain/enqueue 重复下载同一封面。
		if _, loaded := m.soInflight.LoadOrStore(key, struct{}{}); loaded {
			// 已在途（其它 drainer 或章节入队已接管）→ 清除待补，交由在途请求下载。
			m.soPending.Delete(key)
			return true
		}
		select {
		case m.soQueue <- smallObjectRequest{taskID: p.taskID, info: p.info}:
			m.soPending.Delete(key)
		default:
			// 队列仍满：释放在途标记，保留待补，本次排空到此为止。
			m.soInflight.Delete(key)
			return false
		}
		return true
	})
}

// --- Manager 小对象调度 ---

// StartSmallObjectWorkers 启动小对象下载 worker 协程池。
func (m *Manager) StartSmallObjectWorkers(n int) {
	if n <= 0 {
		n = 2
	}
	for i := 0; i < n; i++ {
		m.soWg.Add(1)
		go m.soWorker(i)
	}
	slog.Info("Small-object workers started", "count", n)
}

// StopSmallObjectWorkers 停止小对象下载 worker 协程池。
func (m *Manager) StopSmallObjectWorkers() {
	m.soCancel()
	m.soWg.Wait()
}

// enqueueSmallObjects 检查 task 是否实现了 SmallObjectProvider，有则入队。
// 返回的 tracker 由调用方自行保存/等待（download() 存入 soTracker）；回填路径可忽略返回值。
//
// 去重策略（保证同一小对象只下载一次）：
//  1. 本地文件已存在（非空）→ 跳过下载，直接写回媒体字段并标记完成；
//  2. 其它对象正在下载同一小对象（soInflight 命中）→ 跳过，标记完成。
//
// tracker 始终按 len(items) 计数，每个 item 恰好 MarkDone 一次（跳过或入队），不会挂起主流程。
func (m *Manager) enqueueSmallObjects(t core.Task, obj *model.DownloadObject) *objectTracker {
	// 先补下载之前被丢弃的小对象（best-effort）。
	m.drainPendingSO()

	soc, ok := t.(core.SmallObjectProvider)
	if !ok {
		return nil
	}

	items := soc.SmallObjects(obj)
	if len(items) == 0 {
		return nil
	}

	tracker := newObjectTracker(len(items))

	for _, info := range items {
		key := smallObjectKey(t.ID(), info)
		// 已下载：直接收尾（写回媒体字段），不触碰 soInflight。
		if fileExistsNonEmpty(info.SavePath) {
			finalizeSmallObject(obj, info)
			tracker.MarkDone(nil)
			continue
		}
		// 在途去重：LoadOrStore 命中说明已有请求在下载同一小对象，跳过但同样写回媒体字段
		// （与文件已存在跳过一致，确保只在下载成功时写字段的任务类型也能拿到字段）。
		if _, loaded := m.soInflight.LoadOrStore(key, struct{}{}); loaded {
			finalizeSmallObject(obj, info)
			tracker.MarkDone(nil)
			continue
		}
		req := smallObjectRequest{
			taskID:    t.ID(),
			parentObj: obj,
			info:      info,
			tracker:   tracker,
		}
		select {
		case <-m.soCtx.Done():
			m.soInflight.Delete(key)
			tracker.MarkDone(nil)
		default:
			select {
			case m.soQueue <- req:
			default:
				// 队列满：不阻塞主下载 goroutine，直接记入待补集合并写回媒体字段
				// （不静默丢失），由 drainPendingSO 在后续入队/worker 空闲时补下载。
				m.soInflight.Delete(key)
				m.soPending.Store(key, pendingSO{taskID: t.ID(), info: info})
				finalizeSmallObject(obj, info)
				slog.Warn("Small-object queue full, queued for later retry", logutil.LogKeyTaskID, t.ID(),
					logutil.LogKeyURL, obj.URL, "rel", info.Rel)
				tracker.MarkDone(fmt.Errorf("small-object queue full: %s", info.URL))
			}
		}
	}
	return tracker
}

func (m *Manager) soWorker(id int) {
	defer m.soWg.Done()
	slog.Debug("Small-object worker started", "id", id)
	for {
		select {
		case <-m.soCtx.Done():
			return
		case req := <-m.soQueue:
			m.processSO(req)
			// 刚腾出一个 worker 槽位，顺带补下载待补项。
			m.drainPendingSO()
		}
	}
}

func (m *Manager) processSO(req smallObjectRequest) {
	// 无论成功失败，处理完即释放去重标记，允许后续重新入队。
	defer m.soInflight.Delete(smallObjectKey(req.taskID, req.info))

	// parentObj 可能为 nil（drainPendingSO 的补下载请求 fire-and-forget）。
	parentURL := ""
	if req.parentObj != nil {
		parentURL = req.parentObj.URL
	}
	slog.Debug("Downloading small object", logutil.LogKeyTaskID, req.taskID,
		"parent_url", parentURL, logutil.LogKeyURL, req.info.URL, "rel", req.info.Rel)

	// 下载前再查一次文件是否已就绪（封死 enqueue 与 worker 取出之间的 TOCTOU 窗口）。
	if fileExistsNonEmpty(req.info.SavePath) {
		finalizeSmallObject(req.parentObj, req.info)
		if req.tracker != nil {
			req.tracker.MarkDone(nil)
		}
		return
	}

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
			slog.Warn("Small-object download failed, retrying", logutil.LogKeyTaskID, req.taskID,
				"url", req.info.URL, "rel", req.info.Rel, "attempt", attempt, "backoff", backoff, logutil.LogKeyError, err)
			time.Sleep(backoff)
		}
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		slog.Error("Small-object download failed after retries", logutil.LogKeyTaskID, req.taskID,
			"url", req.info.URL, "rel", req.info.Rel, logutil.LogKeyError, err)
	}

	// 下载成功后将路径写回 parentObj 的固定媒体字段（{rel}_url / {rel}_path），
	// 供前端统一读取；同时保留旧的 local_cover / local_preview 兼容字段。
	if err == nil {
		finalizeSmallObject(req.parentObj, req.info)
	}

	// tracker 可能为 nil（drainPendingSO 的补下载请求为 fire-and-forget）。
	if req.tracker != nil {
		req.tracker.MarkDone(err)
	}
}
