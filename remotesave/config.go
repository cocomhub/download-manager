// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package remotesave 把已完成下载对象的本地文件幂等同步到 sproxy 卷目录。
//
// 口径（用户裁定 2026-11-10）：
//   - 只处理 status=completed 的对象；
//   - 每个文件以 sha256 证明 sdserver 本地文件与 sproxy 文件一致；
//   - 远端已存在但无法用 sha256 证明一致 → conflict，禁止自动覆盖；
//   - 逐文件 sha256 写入 mongo（model.RemoteSaveState）。
package remotesave

import (
	"fmt"
	"path"
	"strings"

	"github.com/cocomhub/download-manager/pkg/configutil"
)

// TaskConfig 是任务级 remote_save 配置（只允许路径，不允许账号）。
type TaskConfig struct {
	Enabled      bool
	Volume       string
	Dir          string
	ObjectDirs   bool
	MirrorDelete bool
	Exclude      []string
}

// forbiddenTaskKeys 是必须集中在全局 sproxy 段的账号/连接字段；任务级出现即判非法。
var forbiddenTaskKeys = []string{
	"api_url", "api_token", "access_key", "access_key_secret", "access_key_id",
	"insecure_tls", "play_base_url", "play_ttl",
}

// ParseTaskConfig 从 task.Extra["remote_save"] 解析任务级配置。
func ParseTaskConfig(extra map[string]any) (TaskConfig, error) {
	cfg := TaskConfig{}
	raw, ok := extra["remote_save"]
	if !ok || raw == nil {
		return cfg, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return cfg, fmt.Errorf("remote_save 必须是对象（map）")
	}
	for _, k := range forbiddenTaskKeys {
		if _, exists := m[k]; exists {
			return cfg, fmt.Errorf("remote_save 不允许配置 %q（sproxy 账号/连接配置必须集中在全局 sproxy 段）", k)
		}
	}
	cfg.Enabled = configutil.GetBool(m, "enabled", false)
	cfg.Volume = strings.TrimSpace(configutil.GetString(m, "volume", ""))
	cfg.Dir = strings.TrimSpace(configutil.GetString(m, "dir", ""))
	cfg.ObjectDirs = configutil.GetBool(m, "object_dirs", false)
	cfg.MirrorDelete = configutil.GetBool(m, "mirror_delete", false)
	cfg.Exclude = stringSlice(m["exclude"])
	if !cfg.Enabled {
		return cfg, nil
	}
	if cfg.Volume == "" {
		return cfg, fmt.Errorf("remote_save.enabled=true 时必须配置 volume")
	}
	if strings.ContainsAny(cfg.Volume, `/\`) || cfg.Volume == "." || cfg.Volume == ".." {
		return cfg, fmt.Errorf("remote_save.volume 非法: %q", cfg.Volume)
	}
	dir, err := normalizeRemoteDir(cfg.Dir)
	if err != nil {
		return cfg, err
	}
	cfg.Dir = dir
	return cfg, nil
}

// normalizeRemoteDir 归一化卷内相对目录：去前导 /、拒绝绝对路径/.. /控制字符。
func normalizeRemoteDir(dir string) (string, error) {
	dir = strings.TrimSpace(strings.ReplaceAll(dir, `\`, "/"))
	dir = strings.TrimPrefix(dir, "/")
	if dir == "" {
		return "", nil
	}
	if strings.ContainsRune(dir, 0) {
		return "", fmt.Errorf("remote_save.dir 含空字节")
	}
	clean := path.Clean(dir)
	if clean == "." {
		return "", nil
	}
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("remote_save.dir 不能逃逸卷根: %q", dir)
	}
	for seg := range strings.SplitSeq(clean, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("remote_save.dir 含非法段: %q", dir)
		}
	}
	return clean, nil
}

// stringSlice 兼容 YAML/JSON/BSON 的 []string/[]any 形态。
func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

// RemotePath 返回任务内相对路径对应的卷内远端路径（A 阶段：<dir>/<rel>）。
func (c TaskConfig) RemotePath(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("空相对路径")
	}
	safe, err := sanitizeRemoteRel(rel)
	if err != nil {
		return "", err
	}
	if c.Dir == "" {
		return safe, nil
	}
	return path.Join(c.Dir, safe), nil
}
