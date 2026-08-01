// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
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

// getCollection 返回指定对象所在合集的所有对象。
// GET /api/objects/{type}/{id}/collection
func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskType := vars["type"]
	idStr := vars["id"]

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return
	}

	objects, err := s.mgr.GetCollectionByID(taskType, id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("collection not found: %v", err))
		return
	}
	if objects == nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	// 确保 task_type metadata
	for _, o := range objects {
		o.EnsureTaskType(taskType)
	}

	w.Header().Set(hdrContentType, "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"objects": objects,
		"total":   len(objects),
	})
}

// getObjectMedia 按对象数字 id 与媒体类型（cover/thumb/preview）返回媒体文件。
// GET /api/objects/{type}/{id}/media/{rel}
//
// 统一从固定字段 {rel}_url / {rel}_path 读取：本地已下载则直接返回文件；
// 本地缺失则懒下载到 {rel}_path 后返回；无本地路径或下载失败则 302 到源 URL。
func (s *Server) getObjectMedia(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskType := vars["type"]
	rel := vars["rel"]
	idStr := vars["id"]

	if !isValidMediaRel(rel) {
		writeJSONError(w, http.StatusBadRequest, "invalid_rel", "rel must be cover, thumb or preview")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id < 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_id", "id must be a non-negative integer")
		return
	}
	obj, err := s.mgr.GetObjectByTypeAndID(taskType, id)
	if err != nil || obj == nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	url := obj.GetMediaURL(rel)
	if url == "" {
		writeJSONError(w, http.StatusNotFound, "media_not_found", "media url not found")
		return
	}
	if localPath := obj.GetMediaPath(rel); localPath != "" {
		if isReadableFile(localPath) {
			serveMediaFile(w, r, localPath)
			return
		}
		// 本地缺失 → 懒下载后 serve
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		if err := download.Get(ctx, url, localPath); err == nil && isReadableFile(localPath) {
			serveMediaFile(w, r, localPath)
			return
		}
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// isValidMediaRel 校验媒体关系类型是否为 cover/thumb/preview。
func isValidMediaRel(rel string) bool {
	switch rel {
	case model.MediaRelCover, model.MediaRelThumb, model.MediaRelPreview:
		return true
	}
	return false
}

// isReadableFile 判断路径是否为非空普通文件。
func isReadableFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Size() > 0
}

// serveMediaFile 按扩展名设置 Content-Type 与缓存头后返回本地文件。
func serveMediaFile(w http.ResponseWriter, r *http.Request, p string) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	switch strings.ToLower(filepath.Ext(p)) {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".webm":
		w.Header().Set("Content-Type", "video/webm")
	}
	http.ServeFile(w, r, p)
}
