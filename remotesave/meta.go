// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// BuildMeta 基于 mongo 对象快照构造 sproxy 文件 meta。
//
// 内置白名单语义字段 + `x_dm_*` 嵌套扩展（sproxy 已支持嵌套 map，只限制完整
// JSON 序列化大小）。必须包含对象 tags、全部 Metadata/Extra，便于在 sproxy
// 元数据弹窗直接查看真实数据。
func BuildMeta(task core.Task, obj *model.DownloadObject, fileRel, remotePath string, size int64, mtime time.Time, sha string, cfg TaskConfig) (map[string]any, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil object")
	}
	snap := obj.Snapshot()
	if snap == nil {
		return nil, fmt.Errorf("nil object snapshot")
	}
	tags := snap.GetTags()
	meta := map[string]any{
		"source":          "local_upload",
		"site":            task.Type(),
		"page_url":        snap.URL,
		"downloaded_at":   mtime.UTC().Format(time.RFC3339Nano),
		"checksum":        sha,
		"x_dm_task_id":    task.ID(),
		"x_dm_task_type":  task.Type(),
		"x_dm_object_url": snap.URL,
		"x_dm_object": map[string]any{
			"task_id":   task.ID(),
			"task_type": task.Type(),
			"url":       snap.URL,
			"id":        snap.GetID(),
			"metadata":  jsonSafe(snap.Metadata),
			"extra":     jsonSafe(snap.Extra),
			"tags":      tags,
		},
		"x_dm_file": map[string]any{
			"path":        fileRel,
			"remote_path": remotePath,
			"size":        size,
			"mtime":       mtime.UTC().Format(time.RFC3339Nano),
			"sha256":      sha,
		},
		"x_dm_remote": map[string]any{
			"volume": cfg.Volume,
			"dir":    cfg.Dir,
		},
	}
	if v := firstNonEmpty(snap.Metadata["code"], snap.Metadata[model.MetadataKeyContentGroup]); v != "" {
		meta["code"] = v
	}
	if v := snap.Metadata[model.MetadataKeyTitle]; v != "" {
		meta["title_ja"] = v
	}
	for _, k := range []string{"maker", "series", "actress", "director", "release_date"} {
		if v := snap.Metadata[k]; v != "" {
			meta[k] = v
		}
	}
	if len(tags) > 0 {
		meta["genres"] = tags
	}
	if b, err := json.Marshal(meta); err != nil {
		return nil, fmt.Errorf("meta 序列化失败: %w", err)
	} else if len(b) > 10<<20 {
		return nil, fmt.Errorf("meta 序列化后超过 10MiB（%d）", len(b))
	}
	return meta, nil
}

// MetaHash 返回 meta 的稳定 JSON 哈希（用于判断是否需要补录）。
func MetaHash(meta map[string]any) (string, error) {
	b, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// jsonSafe 把 BSON 读回形态（bson.D/A、bson.M）递归转为 JSON 原生形态。
func jsonSafe(v any) any {
	switch t := v.(type) {
	case nil, string, bool, int, int32, int64, float32, float64, json.Number:
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = jsonSafe(e)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = e
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = jsonSafe(e)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = e
		}
		return out
	case bson.M:
		return jsonSafe(map[string]any(t))
	case bson.D:
		out := make(map[string]any, len(t))
		for _, e := range t {
			out[e.Key] = jsonSafe(e.Value)
		}
		return out
	case bson.A:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = jsonSafe(e)
		}
		return out
	default:
		// 未知类型（time.Time / 自定义标量）尝试 JSON 往返；失败则降级字符串。
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			return string(b)
		}
		return out
	}
}
