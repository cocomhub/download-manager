// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cocomhub/download-manager/manager"
)

// retryOverviewHandler 返回失败分类聚合统计（GET /api/retry/overview）。
func (s *Server) retryOverviewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(s.mgr.RetryOverview())
}

// RetryCategoryRequest 是按分类批量重试的请求体。
type RetryCategoryRequest struct {
	Category string `json:"category"`
}

// retryCategoryHandler 按失败分类批量重试（POST /api/retry/retry-category）。
// 分类语义：
//   - http_4xx：永久失败（源站明确拒绝），重试前应人工确认——这里仍提供按需重试能力，
//     但 UI 会提示该分类一般不值得重试。
//   - http_5xx / timeout：可重试，重置失败对象回 pending 走正常调度。
//   - other：其余失败，重置 failed 对象（failed_permanent 的「other」多为重试耗尽，
//     由 hourly 自动重试兜底，不在此重置）。
func (s *Server) retryCategoryHandler(w http.ResponseWriter, r *http.Request) {
	var req RetryCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "invalid request body")
		return
	}
	statuses, ok := retryStatusesForCategory(req.Category)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid_category",
			fmt.Sprintf("unknown retry category: %s", req.Category))
		return
	}
	retried, err := s.mgr.RetryAllFailedStatus(statuses)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeUpdateFailed, fmt.Sprintf("failed to retry category: %v", err))
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"category": req.Category,
		"retried":  retried,
	})
}

// retryStatusesForCategory 返回该分类应重置的存储状态集合。
// 除 http_4xx 外，分类重试只重置普通 failed（可重试）；failed_permanent
// 由 hourly 自动重试（retryFailedPermanent）按最少失败批次兜底，避免手动全量冲击源站。
func retryStatusesForCategory(category string) ([]string, bool) {
	switch category {
	case manager.FailureCategoryHTTP4xx:
		return []string{"failed_permanent"}, true
	case manager.FailureCategoryHTTP5xx, manager.FailureCategoryTimeout, manager.FailureCategoryOther:
		return []string{"failed"}, true
	}
	return nil, false
}
