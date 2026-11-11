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
	ObjectURL    string    `json:"object_url,omitempty"`
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
	flushMu sync.Mutex
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
	// 旧版本以 rel 为 key；统一迁移为 objectURL+rel，避免多对象同名文件互相覆盖。
	migrated := make(map[string]ManifestEntry, len(m.entries))
	for key, e := range m.entries {
		if e.Path == "" {
			e.Path = key
		}
		k := manifestKey(e.ObjectURL, e.Path)
		if _, exists := migrated[k]; exists {
			continue
		}
		migrated[k] = e
	}
	if len(migrated) != len(m.entries) {
		m.entries = migrated
		m.dirty = true
	} else {
		// 即使条目数量相同，也可能 key 还是旧格式；逐 key 判断后迁移。
		changed := false
		for key, e := range m.entries {
			if key != manifestKey(e.ObjectURL, e.Path) {
				changed = true
				break
			}
		}
		if changed {
			m.entries = migrated
			m.dirty = true
		}
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

// manifestKey 用对象 URL + 相对路径隔离不同对象的同名文件。
func manifestKey(objectURL, rel string) string {
	return objectURL + "\x00" + rel
}

// GetFor 返回指定对象的 entry 副本；兼容旧版以 rel 为 key 的 manifest。
func (m *Manifest) GetFor(objectURL, rel string) (ManifestEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[manifestKey(objectURL, rel)]; ok {
		return e, true
	}
	if e, ok := m.entries[rel]; ok && (e.ObjectURL == "" || e.ObjectURL == objectURL) {
		return e, true
	}
	for _, e := range m.entries {
		if e.Path == rel && (e.ObjectURL == "" || e.ObjectURL == objectURL) {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// Get 是测试/兼容入口：按相对路径查找（同路径多对象时返回任意一条）。
func (m *Manifest) Get(rel string) (ManifestEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[rel]; ok {
		return e, true
	}
	for _, e := range m.entries {
		if e.Path == rel {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// PutFor 用对象 URL + 路径作为 key 写入。
func (m *Manifest) PutFor(objectURL string, e ManifestEntry) {
	if e.ObjectURL == "" {
		e.ObjectURL = objectURL
	}
	if objectURL == "" {
		objectURL = e.ObjectURL
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]ManifestEntry{}
	}
	e.UpdatedAt = time.Now().UTC()
	m.entries[manifestKey(objectURL, e.Path)] = e
	delete(m.entries, e.Path) // 清理旧格式同名 key
	m.gen++
	m.dirty = true
}

// DeleteFor 删除指定对象的 entry。
func (m *Manifest) DeleteFor(objectURL, rel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := manifestKey(objectURL, rel)
	if _, ok := m.entries[key]; ok {
		delete(m.entries, key)
		m.gen++
		m.dirty = true
	}
	if e, ok := m.entries[rel]; ok && (e.ObjectURL == "" || e.ObjectURL == objectURL) {
		delete(m.entries, rel)
		m.gen++
		m.dirty = true
	}
}

// Put 写入/更新 entry（仅标记 dirty，flush 时落盘）。
func (m *Manifest) Put(e ManifestEntry) {
	m.PutFor(e.ObjectURL, e)
}

// Delete 删除 entry（镜像清理成功后调用）。
func (m *Manifest) Delete(rel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for k, e := range m.entries {
		if e.Path == rel || k == rel {
			delete(m.entries, k)
			found = true
		}
	}
	if found {
		m.gen++
		m.dirty = true
	}
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
	m.flushMu.Lock()
	defer m.flushMu.Unlock()
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
	byPath := make(map[string]ManifestEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	for _, p := range paths {
		e, ok := byPath[p]
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
