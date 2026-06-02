// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cocomhub/download-manager/config"
)

// ConfigService manages configuration persistence, backups, diffs, and metadata.
// Runtime hot-reload side effects (downloader rebuild, logger re-init, worker
// adjustment) remain on Manager; this service handles only file I/O and state.
type ConfigService struct {
	cfg    *config.Config
	cfgVal atomic.Value
}

// NewConfigService creates a ConfigService with the initial config.
func NewConfigService(cfg *config.Config) *ConfigService {
	cs := &ConfigService{cfg: cfg}
	cs.cfgVal.Store(cfg)
	return cs
}

// GetConfig returns the current config from the atomic value, or fallback.
func (cs *ConfigService) GetConfig() *config.Config {
	if v := cs.cfgVal.Load(); v != nil {
		return v.(*config.Config)
	}
	return cs.cfg
}

// StoreConfig updates the in-memory config state.
func (cs *ConfigService) StoreConfig(cfg *config.Config) {
	cs.cfg = cfg
	cs.cfgVal.Store(cfg)
}

// --- Backup operations ---

func (cs *ConfigService) writeConfigBackup() (string, error) {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("config_%s.yaml", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	cur := config.GetConfigFilePath()
	data, err := os.ReadFile(cur)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return name, nil
}

func (cs *ConfigService) ListConfigBackups() ([]map[string]string, error) {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []map[string]string{}, nil
	}
	var res []map[string]string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "config_") && strings.HasSuffix(name, ".yaml") {
			item := map[string]string{
				"filename": name,
				"path":     filepath.Join(dir, name),
			}
			metaPath := filepath.Join(dir, name+".meta.json")
			if data, err := os.ReadFile(metaPath); err == nil {
				item["meta"] = string(data)
			}
			res = append(res, item)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i]["filename"] > res[j]["filename"]
	})
	return res, nil
}

func (cs *ConfigService) DeleteConfigBackup(filename string) error {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	path := filepath.Join(dir, filename)
	meta := path + ".meta.json"
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	_ = os.Remove(meta)
	return nil
}

func (cs *ConfigService) RollbackLoad(filename string) (*config.Config, error) {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup: %w", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid backup content: %w", err)
	}
	return &cfg, nil
}

// --- Diff operations ---

func (cs *ConfigService) DiffConfigFiles(left, right string) (map[string]any, error) {
	leftCfg, leftYml, err := cs.readDiffSide(left)
	if err != nil {
		return nil, fmt.Errorf("left side: %w", err)
	}
	rightCfg, rightYml, err := cs.readDiffSide(right)
	if err != nil {
		return nil, fmt.Errorf("right side: %w", err)
	}
	diff := leftCfg.Diff(*rightCfg)
	return map[string]any{
		"left":       left,
		"right":      right,
		"left_yaml":  string(leftYml),
		"right_yaml": string(rightYml),
		"changes":    diff,
	}, nil
}

func (cs *ConfigService) DiffConfigFilesOpts(left, right string, ignoreWS, ignoreComments bool) (map[string]any, error) {
	res, err := cs.DiffConfigFiles(left, right)
	if err != nil {
		return nil, err
	}
	if ignoreWS || ignoreComments {
		res["left_norm"] = normalizeYAML(res["left_yaml"].(string), ignoreWS, ignoreComments)
		res["right_norm"] = normalizeYAML(res["right_yaml"].(string), ignoreWS, ignoreComments)
	}
	return res, nil
}

func (cs *ConfigService) readDiffSide(ref string) (*config.Config, []byte, error) {
	var cfg config.Config
	var yml []byte
	var err error
	if ref == "current" || ref == "" {
		yml, err = os.ReadFile(config.GetConfigFilePath())
		if err != nil {
			return nil, nil, fmt.Errorf("read current config failed: %w", err)
		}
		if err := yaml.Unmarshal(yml, &cfg); err != nil {
			return nil, nil, fmt.Errorf("parse current config failed: %w", err)
		}
	} else {
		p := filepath.Join(config.GetWorkDir(), "config_backups", ref)
		yml, err = os.ReadFile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("read backup failed: %w", err)
		}
		if err := yaml.Unmarshal(yml, &cfg); err != nil {
			return nil, nil, fmt.Errorf("parse backup failed: %w", err)
		}
	}
	return &cfg, yml, nil
}

func normalizeYAML(src string, ignoreWS, ignoreComments bool) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if ignoreComments {
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "#") {
				continue
			}
		}
		if ignoreWS {
			l = strings.TrimRight(l, " \t")
			l = strings.ReplaceAll(l, "\t", "    ")
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// --- Tag / Note metadata ---

func (cs *ConfigService) AddConfigTag(filename, tag string) error {
	if tag == "" {
		return fmt.Errorf("tag is empty")
	}
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	meta := filepath.Join(dir, filename+".meta.json")
	var obj struct {
		Tags  []string `json:"tags"`
		Notes []struct {
			Message   string `json:"message"`
			Author    string `json:"author"`
			Timestamp int64  `json:"timestamp"`
		} `json:"notes"`
	}
	if data, err := os.ReadFile(meta); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	if slices.Contains(obj.Tags, tag) {
		return nil
	}
	obj.Tags = append(obj.Tags, tag)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(obj)
	return os.WriteFile(meta, buf.Bytes(), 0644)
}

func (cs *ConfigService) AddConfigNote(filename, message, author string) error {
	if message == "" {
		return fmt.Errorf("message is empty")
	}
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	meta := filepath.Join(dir, filename+".meta.json")
	var obj struct {
		Tags  []string `json:"tags"`
		Notes []struct {
			Message   string `json:"message"`
			Author    string `json:"author"`
			Timestamp int64  `json:"timestamp"`
		} `json:"notes"`
	}
	if data, err := os.ReadFile(meta); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	obj.Notes = append(obj.Notes, struct {
		Message   string `json:"message"`
		Author    string `json:"author"`
		Timestamp int64  `json:"timestamp"`
	}{Message: message, Author: author, Timestamp: time.Now().Unix()})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(obj)
	return os.WriteFile(meta, buf.Bytes(), 0644)
}

// --- Config file writing with comment preservation ---

func (cs *ConfigService) WriteConfigWithComments(cfg *config.Config) error {
	path := config.GetConfigFilePath()
	origData, err := os.ReadFile(path)
	if err != nil {
		return config.Save(path, cfg)
	}
	var origRoot yaml.Node
	if err := yaml.Unmarshal(origData, &origRoot); err != nil {
		return config.Save(path, cfg)
	}
	var newRoot yaml.Node
	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return config.Save(path, cfg)
	}
	if err := yaml.Unmarshal(newData, &newRoot); err != nil {
		return config.Save(path, cfg)
	}
	if len(origRoot.Content) == 0 || origRoot.Content[0].Kind != yaml.MappingNode {
		return config.Save(path, cfg)
	}
	dst := origRoot.Content[0]
	src := newRoot.Content[0]
	for _, key := range []string{"server", "log", "mongo", "downloader", "task_scan", "contexts"} {
		_, val, _ := mapGet(src, key)
		if val != nil {
			mapSet(dst, key, val)
		}
	}
	_, srcTasks, _ := mapGet(src, "tasks")
	_, dstTasks, _ := mapGet(dst, "tasks")
	if srcTasks != nil {
		if dstTasks == nil {
			dstTasks = &yaml.Node{Kind: yaml.SequenceNode}
			mapSet(dst, "tasks", dstTasks)
		}
		dstMap := map[string]*yaml.Node{}
		for _, dItem := range dstTasks.Content {
			if dItem.Kind == yaml.MappingNode {
				_, idNode, _ := mapGet(dItem, "id")
				if idNode != nil {
					dstMap[idNode.Value] = dItem
				}
			}
		}
		for _, sItem := range srcTasks.Content {
			if sItem.Kind != yaml.MappingNode {
				continue
			}
			_, sId, _ := mapGet(sItem, "id")
			if sId == nil {
				continue
			}
			if dItem, ok := dstMap[sId.Value]; ok {
				for _, k := range []string{"type", "save_dir", "storage", "storage_context", "extra"} {
					_, sVal, _ := mapGet(sItem, k)
					if sVal != nil {
						mapSet(dItem, k, sVal)
					}
				}
			} else {
				dstTasks.Content = append(dstTasks.Content, sItem)
			}
		}
	}
	return writeYAMLToFile(path, &origRoot)
}

// --- Package-level helpers (shared with old config_mgr.go until fully migrated) ---

func writeYAMLToFile(path string, root *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return err
	}
	_ = enc.Close()
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func mapGet(node *yaml.Node, key string) (*yaml.Node, *yaml.Node, int) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, nil, -1
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Value == key {
			return k, v, i
		}
	}
	return nil, nil, -1
}

func mapSet(dst *yaml.Node, key string, newVal *yaml.Node) {
	_, oldVal, idx := mapGet(dst, key)
	if idx >= 0 {
		newVal.HeadComment = oldVal.HeadComment
		newVal.LineComment = oldVal.LineComment
		newVal.FootComment = oldVal.FootComment
		dst.Content[idx+1] = newVal
		return
	}
	newKey := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	dst.Content = append(dst.Content, newKey, newVal)
}
