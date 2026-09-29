// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// 失败分类：重试中心把失败记录归类为四类，供聚合统计与按类批量重试。
const (
	FailureCategoryHTTP4xx = "http_4xx" // 永久：客户端错误（404/403 等），重试无意义
	FailureCategoryHTTP5xx = "http_5xx" // 可重试：服务端错误（500/503 等），源站恢复后可下
	FailureCategoryTimeout = "timeout"  // 可重试：超时类错误（context deadline exceeded 等）
	FailureCategoryOther   = "other"    // 其它：未识别错误
)

// categorizeFailure 根据失败错误串与永久标记派生分类。
// 优先级：Permanent（重试耗尽/ErrNoTry）→ 4xx 永久 → 5xx → 超时 → 其它。
// 说明：FailureRecord 目前只存错误串与 permanent 布尔（无结构化状态码），
// 因此 4xx/5xx/超时通过错误串启发式匹配（HTTP 状态码 / deadline / timeout 关键字）；
// 若未来 recordFailure 记录结构化状态码，可替换为精确分类。
func categorizeFailure(errStr string, permanent bool) string {
	s := strings.ToLower(errStr)
	if permanent {
		if strings.Contains(s, "http 4") {
			return FailureCategoryHTTP4xx
		}
		return FailureCategoryOther
	}
	if strings.Contains(s, "http 4") {
		return FailureCategoryHTTP4xx
	}
	if strings.Contains(s, "http 5") {
		return FailureCategoryHTTP5xx
	}
	if strings.Contains(s, "deadline") || strings.Contains(s, "timeout") {
		return FailureCategoryTimeout
	}
	return FailureCategoryOther
}

// RecordFailure 向环形缓冲记录一条失败（导出包装，供 API 层与外部组件写入失败记录）。
func (m *Manager) RecordFailure(taskID, url, errStr string, attempt int, permanent bool) {
	m.recordFailure(taskID, url, errStr, attempt, permanent)
}

// RetryOverview 是 GET /api/retry/overview 的聚合统计结果。
type RetryOverview struct {
	Categories  map[string]int `json:"categories"`
	TotalFailed int            `json:"total_failed"`
	Retryable   int            `json:"retryable"`
	Permanent   int            `json:"permanent"`
}

// RetryOverview 聚合环形缓冲区的失败记录：按分类计数，并统计可重试/永久数量。
func (m *Manager) RetryOverview() RetryOverview {
	ov := RetryOverview{
		Categories: map[string]int{
			FailureCategoryHTTP4xx: 0,
			FailureCategoryHTTP5xx: 0,
			FailureCategoryTimeout: 0,
			FailureCategoryOther:   0,
		},
	}

	m.failureMu.Lock()
	defer m.failureMu.Unlock()
	for i := range min(m.failureWriteIdx, m.maxFailures) {
		idx := (m.failureWriteIdx - 1 - i + m.maxFailures) % m.maxFailures
		r := m.failureRecords[idx]
		if r.TaskID == "" {
			continue
		}
		ov.TotalFailed++
		if r.Permanent {
			ov.Permanent++
		} else {
			ov.Retryable++
		}
		cat := categorizeFailure(r.Error, r.Permanent)
		ov.Categories[cat]++
	}
	return ov
}

// RetryAllFailed 重置所有任务中指定状态的失败对象为 pending（按分类重试的支撑）。
// 与 ObjectController.RetryAllFailed 语义一致，但跨任务聚合。
func (m *Manager) RetryAllFailedStatus(statuses []string) (int, error) {
	retried := 0
	var firstErr error
	m.tasks.Range(func(_, value any) bool {
		t := value.(core.Task)
		objs, err := m.collectTaskObjects(t, &core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: statuses,
			},
		}, 200)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return true
		}
		for _, obj := range objs {
			if obj.GetStatus() == model.StatusCompleted {
				continue
			}
			t.UpdateStatus(obj, model.StatusPending, nil)
			obj.SetProgress(0)
			m.getOrCreateMetrics(t.ID()).retried.Add(1)
			retried++
		}
		return true
	})
	if retried > 0 {
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	return retried, firstErr
}
