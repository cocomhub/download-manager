// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// systemShutdownHandler 实现「完成当前下载任务后退出」。
//
// 语义（用户确认 2026-09-28）：
//   - 仅等「真正正在 Download()」的对象跑完（manager.inflight 判据）；
//   - 已入队未开始的对象丢弃，保持 pending 状态（下次启动可续跑）；
//   - 无超时上限，直到在途耗尽；
//   - 本 handler 立即返回 202，退出在后台异步进行。
//
// 跨站表单防护：带 Origin 头且与请求 Host 不同源 → 403，防止同网任意主机
// 或恶意页面用 <form method=POST> 触发 shutdown。无 Origin 头（curl/非浏览器）放行。
//
// 响应 202：已受理排空，进程将在当前下载完成后自行退出。
func (s *Server) systemShutdownHandler(w http.ResponseWriter, r *http.Request) {
	// Origin 同源校验：Origin 存在但与 r.Host 不一致（含解析失败）→ 拒绝。
	if origin := r.Header.Get("Origin"); origin != "" {
		oh, err := url.Parse(origin)
		if err != nil || oh.Host != r.Host {
			writeJSONError(w, http.StatusForbidden, "cross_site_origin",
				"cross-site shutdown request denied")
			return
		}
	}

	started := s.mgr.StartDrain()
	if s.drainTrigger != nil {
		s.drainTrigger()
	}
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)
	status := "draining"
	if !started {
		// 已处于排空/停止中：同样返回 202，幂等。
		status = "already_draining"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}
