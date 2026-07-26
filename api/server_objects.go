// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// getObjectByTypeAndID 返回单个下载对象详情。
// GET /api/objects/{type}/{id}
func (s *Server) getObjectByTypeAndID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskType := vars["type"]
	idStr := vars["id"]

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id < 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "id must be a non-negative integer")
		return
	}

	obj, err := s.mgr.GetObjectByTypeAndID(taskType, id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("object not found: %v", err))
		return
	}
	if obj == nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	// 确保 task_type metadata 存在
	obj.EnsureTaskType(taskType)

	w.Header().Set(hdrContentType, "application/json")
	json.NewEncoder(w).Encode(obj)
}
