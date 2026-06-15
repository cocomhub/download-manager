// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocomhub/download-manager/task"
)

// metricsHandler returns collected metrics from the manager.
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	metrics := s.mgr.CollectMetrics()
	json.NewEncoder(w).Encode(metrics)
}

// failuresHandler returns failure records.
func (s *Server) failuresHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := r.URL.Query().Get("task_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	result := s.mgr.GetFailures(taskID, limit)
	json.NewEncoder(w).Encode(result)
}

// aggregateObjects aggregates download objects across tasks.
func (s *Server) aggregateObjects(w http.ResponseWriter, r *http.Request) {
	page := int64(1)
	limit := int64(50)
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.ParseInt(pStr, 10, 64); err == nil && p > 0 {
			page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if lStr == "all" {
			limit = -1
		} else if l, err := strconv.ParseInt(lStr, 10, 64); err == nil {
			limit = l
		}
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortBy := r.URL.Query().Get("sort")
	status := r.URL.Query().Get("status")
	groupBy := r.URL.Query().Get("group_by")
	typesParam := strings.TrimSpace(r.URL.Query().Get("types"))
	var types []string
	if typesParam != "" {
		for t := range strings.SplitSeq(typesParam, ",") {
			t = task.NormalizeType(t)
			if t != "" {
				types = append(types, t)
			}
		}
	}
	var (
		res map[string]any
		err error
	)
	if groupBy == "content" {
		res, err = s.mgr.AggregateByContent(page, limit, search, sortBy, status, types)
	} else {
		res, err = s.mgr.AggregateObjects(page, limit, search, sortBy, status, types)
	}
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "aggregate_failed", fmt.Sprintf("Failed to aggregate objects: %v", err))
		return
	}
	json.NewEncoder(w).Encode(res)
}
