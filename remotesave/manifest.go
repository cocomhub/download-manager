// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/model"
)

// ManifestEntry 是远程保存的逐文件操作缓存（不是唯一事实源：mongo 中另有逐文件 sha256）。
type ManifestEntry struct {
	Path         string    `json:"path"`
	RemotePath   string    `json:"remote_path"`
	Size         int64     `json:"size"`
	MTimeNs      int64     `json:"mtime_ns"`
	SHA256       string    `json:"sha256"`
	RemoteSHA256 string    `json:"remote_sha256"`
	MetaHash     string    `json:"meta_hash,omitempty"`
	Origin       string    `json:"origin,omitempty"` // local | cloud
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Manifest 是单任务的逐文件操作缓存，落盘为 JSON（原子替换，批量 flush）。
type Manifest struct {
	mu      sync.Mutex
	path    string
	entries map[string]ManifestEntry
	dirty   bool
	gen     uint64
	saved   uint64
}

// LoadManifest 加载任务 manifest；不存在返回空；损坏时改名隔离并从空开始。
func LoadManifest(dir, taskID string) (*Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, safeManifestName(taskID)+".json")
	m := &Manifest{path: p, entries: map[string]ManifestEntry{}}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m.entries); err != nil {
		_ = os.Rename(p, fmt.Sprintf("%s.corrupt.%d", p, time.Now().Unix()))
		m.entries = map[string]ManifestEntry{}
		return m, fmt.Errorf("remote_save manifest 损坏，已隔离并重建: %w", err)
	}
	if m.entries == nil {
		m.entries = map[string]ManifestEntry{}
	}
	return m, nil
}

func safeManifestName(taskID string) string {
	var b strings.Builder
	for _, r := range taskID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "_task"
	}
	return out
}

// Get 返回 entry 副本。
func (m *Manifest) Get(rel string) (ManifestEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[rel]
	return e, ok
}

// Put 写入/更新 entry（仅标记 dirty，flush 时落盘）。
func (m *Manifest) Put(e ManifestEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]ManifestEntry{}
	}
	e.UpdatedAt = time.Now().UTC()
	m.entries[e.Path] = e
	m.gen++
	m.dirty = true
}

// Snapshot 返回所有 entry 副本（用于构建对象状态）。
func (m *Manifest) Snapshot() map[string]ManifestEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]ManifestEntry, len(m.entries))
	maps.Copy(out, m.entries)
	return out
}

// Flush 原子落盘（无变更时 no-op）。
func (m *Manifest) Flush() error {
	m.mu.Lock()
	if !m.dirty || m.gen == m.saved {
		m.mu.Unlock()
		return nil
	}
	genAt := m.gen
	raw, err := json.Marshal(m.entries)
	p := m.path
	m.mu.Unlock()
	if err != nil {
		m.mu.Lock()
		m.dirty = true
		m.mu.Unlock()
		return err
	}
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, ".remote-save-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		m.mu.Lock()
		m.dirty = true
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	if m.gen == genAt {
		m.saved = genAt
		m.dirty = false
	}
	m.mu.Unlock()
	return nil
}

// ToModelFiles 把 manifest 中指定对象的文件转成 mongo 逐文件状态（含 sha256）。
func ToModelFiles(entries map[string]ManifestEntry, paths []string, limit int) ([]model.RemoteFileState, bool) {
	out := make([]model.RemoteFileState, 0, len(paths))
	truncated := false
	for _, p := range paths {
		e, ok := entries[p]
		if !ok {
			continue
		}
		if limit > 0 && len(out) >= limit {
			truncated = true
			break
		}
		out = append(out, model.RemoteFileState{
			Path:         e.Path,
			RemotePath:   e.RemotePath,
			Size:         e.Size,
			MTime:        time.Unix(0, e.MTimeNs).UTC(),
			SHA256:       e.SHA256,
			RemoteSHA256: e.RemoteSHA256,
			Status:       e.Status,
			Reason:       e.Reason,
			UpdatedAt:    e.UpdatedAt,
		})
	}
	return out, truncated
}
