// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cocomhub/download-manager/config"
)

// getTaskTypeDefaults returns the current task type defaults configuration.
// GET /api/config/task-type-defaults
func (s *Server) getTaskTypeDefaults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(hdrContentType, "application/json")
	defaults := s.mgr.GetTaskTypeDefaults()
	json.NewEncoder(w).Encode(defaults)
}

// updateTaskTypeDefaults updates the task type defaults configuration.
// PUT /api/config/task-type-defaults
func (s *Server) updateTaskTypeDefaults(w http.ResponseWriter, r *http.Request) {
	var req map[string]config.TaskTypeDefault
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}
	if err := s.mgr.SetTaskTypeDefaults(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeUpdateFailed, fmt.Sprintf("Failed to update task type defaults: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
