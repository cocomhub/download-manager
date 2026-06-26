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
	page := parsePageParam(r)
	limit := parseLimitParam(r)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortBy := r.URL.Query().Get("sort")
	status := r.URL.Query().Get("status")
	groupBy := r.URL.Query().Get("group_by")
	types := parseTypesParam(r)

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

// parsePageParam reads and validates the "page" query parameter.
func parsePageParam(r *http.Request) int64 {
	pStr := r.URL.Query().Get("page")
	if pStr == "" {
		return 1
	}
	p, err := strconv.ParseInt(pStr, 10, 64)
	if err != nil || p <= 0 {
		return 1
	}
	return p
}

// parseLimitParam reads the "limit" query parameter, supporting "all" for -1.
func parseLimitParam(r *http.Request) int64 {
	lStr := r.URL.Query().Get("limit")
	if lStr == "" {
		return 50
	}
	if lStr == "all" {
		return -1
	}
	l, err := strconv.ParseInt(lStr, 10, 64)
	if err != nil {
		return 50
	}
	return l
}

// parseTypesParam reads and normalizes the comma-separated "types" query parameter.
func parseTypesParam(r *http.Request) []string {
	typesParam := strings.TrimSpace(r.URL.Query().Get("types"))
	if typesParam == "" {
		return nil
	}
	var types []string
	for t := range strings.SplitSeq(typesParam, ",") {
		t = task.NormalizeType(t)
		if t == "" {
			continue
		}
		types = append(types, t)
	}
	return types
}
