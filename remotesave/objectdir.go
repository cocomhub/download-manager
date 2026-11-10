// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strconv"
	"strings"

	"github.com/cocomhub/download-manager/model"
)

// ObjectKey 返回对象的稳定目录键：code → content_group → 数字 ID → title。
//
// tktube 的 content_group 就是番号（变体共享同目录）；njavtv 用 code；hanime
// 无番号时用视频 ID。未知标题用短哈希兜底，避免不同对象误合并。
func ObjectKey(obj *model.DownloadObject) string {
	if obj == nil {
		return "unknown"
	}
	snap := obj.Snapshot()
	if snap == nil {
		return "unknown"
	}
	if v := strings.TrimSpace(snap.Metadata["code"]); v != "" {
		return sanitizeRemoteSegment(v, true)
	}
	if v := strings.TrimSpace(snap.Metadata[model.MetadataKeyContentGroup]); v != "" && !strings.HasPrefix(v, "unknown+") {
		return sanitizeRemoteSegment(v, true)
	}
	if id := snap.GetID(); id != 0 {
		return sanitizeRemoteSegment(strconv.FormatInt(id, 10), true)
	}
	if v := strings.TrimSpace(snap.Metadata[model.MetadataKeyTitle]); v != "" {
		return sanitizeRemoteSegment(v, true)
	}
	sum := sha256.Sum256([]byte(snap.URL))
	return "unknown-" + hex.EncodeToString(sum[:4])
}

// objectRemotePaths 为同一对象的文件计算远端路径（object_dirs 时按番号目录归档）。
// 同名/大小写冲突追加确定性短哈希后缀，保证每次运行一致。
func objectRemotePaths(obj *model.DownloadObject, files []localFile, rels []string, cfg TaskConfig) map[string]string {
	out := make(map[string]string, len(rels))
	if !cfg.ObjectDirs {
		for _, rel := range rels {
			if p, err := cfg.RemotePath(rel); err == nil {
				out[rel] = p
			}
		}
		return out
	}
	key := ObjectKey(obj)
	used := map[string]string{} // lower remote -> rel
	for _, rel := range rels {
		base := sanitizeRemoteSegment(path.Base(rel), false)
		candidate := path.Join(cfg.Dir, key, base)
		lower := strings.ToLower(candidate)
		if prev, exists := used[lower]; exists && prev != rel {
			candidate = path.Join(cfg.Dir, key, withHashSuffix(base, rel))
			lower = strings.ToLower(candidate)
		}
		used[lower] = rel
		out[rel] = candidate
	}
	return out
}

// withHashSuffix 在扩展名前追加 ~<8hex>，用于同名冲突消歧。
func withHashSuffix(base, rel string) string {
	sum := sha256.Sum256([]byte(rel))
	suffix := "~" + hex.EncodeToString(sum[:4])
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return name + suffix + ext
}
