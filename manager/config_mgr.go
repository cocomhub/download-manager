package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"download-manager/config"
)

type AuditInfo struct {
	Author  string
	Source  string
	Message string
}

func (m *Manager) GetConfig() *config.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *Manager) writeConfigBackup() (string, error) {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("config_%s.yaml", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	// Copy current config file bytes to backup to preserve comments and exact original
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

func (m *Manager) ListConfigBackups() ([]map[string]string, error) {
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

func (m *Manager) DeleteConfigBackup(filename string) error {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	path := filepath.Join(dir, filename)
	meta := path + ".meta.json"
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	_ = os.Remove(meta)
	return nil
}

func (m *Manager) RollbackConfig(filename string, audit *AuditInfo) error {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("invalid backup content: %w", err)
	}
	return m.UpdateConfig(&cfg, audit)
}

func (m *Manager) DiffConfigFiles(left, right string) (map[string]interface{}, error) {
	var leftCfg, rightCfg config.Config
	var leftYml, rightYml []byte
	var err error
	if left == "current" || left == "" {
		raw, err := os.ReadFile(config.GetConfigFilePath())
		if err != nil {
			return nil, fmt.Errorf("read current config failed: %w", err)
		}
		leftYml = raw
		if err := yaml.Unmarshal(leftYml, &leftCfg); err != nil {
			return nil, fmt.Errorf("parse current config failed: %w", err)
		}
	} else {
		lp := filepath.Join(config.GetWorkDir(), "config_backups", left)
		leftYml, err = os.ReadFile(lp)
		if err != nil {
			return nil, fmt.Errorf("read left backup failed: %w", err)
		}
		if err := yaml.Unmarshal(leftYml, &leftCfg); err != nil {
			return nil, fmt.Errorf("parse left backup failed: %w", err)
		}
	}
	if right == "current" || right == "" {
		raw, err := os.ReadFile(config.GetConfigFilePath())
		if err != nil {
			return nil, fmt.Errorf("read current config failed: %w", err)
		}
		rightYml = raw
		if err := yaml.Unmarshal(rightYml, &rightCfg); err != nil {
			return nil, fmt.Errorf("parse current config failed: %w", err)
		}
	} else {
		rp := filepath.Join(config.GetWorkDir(), "config_backups", right)
		rightYml, err = os.ReadFile(rp)
		if err != nil {
			return nil, fmt.Errorf("read right backup failed: %w", err)
		}
		if err := yaml.Unmarshal(rightYml, &rightCfg); err != nil {
			return nil, fmt.Errorf("parse right backup failed: %w", err)
		}
	}
	diff := leftCfg.Diff(rightCfg)
	return map[string]interface{}{
		"left":       left,
		"right":      right,
		"left_yaml":  string(leftYml),
		"right_yaml": string(rightYml),
		"changes":    diff,
	}, nil
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

func (m *Manager) DiffConfigFilesOpts(left, right string, ignoreWS, ignoreComments bool) (map[string]interface{}, error) {
	res, err := m.DiffConfigFiles(left, right)
	if err != nil {
		return nil, err
	}
	if ignoreWS || ignoreComments {
		res["left_norm"] = normalizeYAML(res["left_yaml"].(string), ignoreWS, ignoreComments)
		res["right_norm"] = normalizeYAML(res["right_yaml"].(string), ignoreWS, ignoreComments)
	}
	return res, nil
}

func (m *Manager) AddConfigTag(filename, tag string) error {
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
	for _, t := range obj.Tags {
		if t == tag {
			return nil
		}
	}
	obj.Tags = append(obj.Tags, tag)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(obj)
	return os.WriteFile(meta, buf.Bytes(), 0644)
}

func (m *Manager) AddConfigNote(filename, message, author string) error {
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
		// copy comments from old to new
		newVal.HeadComment = oldVal.HeadComment
		newVal.LineComment = oldVal.LineComment
		newVal.FootComment = oldVal.FootComment
		dst.Content[idx+1] = newVal
		return
	}
	// append new key/value
	newKey := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	dst.Content = append(dst.Content, newKey, newVal)
}

func (m *Manager) writeConfigWithComments(cfg *config.Config) error {
	path := config.GetConfigFilePath()
	origData, err := os.ReadFile(path)
	if err != nil {
		// fallback to normal save
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
	// origRoot.Content[0] should be mapping
	if len(origRoot.Content) == 0 || origRoot.Content[0].Kind != yaml.MappingNode {
		return config.Save(path, cfg)
	}
	dst := origRoot.Content[0]
	src := newRoot.Content[0]
	// Merge top-level keys while preserving comments
	for _, key := range []string{"server", "log", "mongo", "downloader", "task_scan"} {
		_, val, _ := mapGet(src, key)
		if val != nil {
			mapSet(dst, key, val)
		}
	}
	// Special handling for tasks: preserve per-item comments
	_, srcTasks, _ := mapGet(src, "tasks")
	_, dstTasks, _ := mapGet(dst, "tasks")
	if srcTasks != nil {
		if dstTasks == nil {
			dstTasks = &yaml.Node{Kind: yaml.SequenceNode}
			mapSet(dst, "tasks", dstTasks)
		}
		// Build id->node map for dst
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
				for _, k := range []string{"type", "save_dir", "storage", "extra"} {
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
