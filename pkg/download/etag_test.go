// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"os"
	"testing"
	"time"
)

// mockFileEntry 模拟文件条目
type mockFileEntry struct {
	size int64
}

// mockFileInfo 模拟 os.FileInfo
type mockFileInfo struct {
	size int64
}

func (m *mockFileInfo) Name() string       { return "" }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

// mockFileSystem 模拟文件系统状态
type mockFileSystem struct {
	files map[string]*mockFileEntry
}

func newMockFS() *mockFileSystem {
	return &mockFileSystem{files: make(map[string]*mockFileEntry)}
}

func (m *mockFileSystem) addFile(path string, size int64) {
	m.files[path] = &mockFileEntry{size: size}
}

// mockStat 返回 FileStatFunc。
func (m *mockFileSystem) mockStat() FileStatFunc {
	return func(path string) (os.FileInfo, error) {
		e, ok := m.files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return &mockFileInfo{size: e.size}, nil
	}
}

// mockChecksumer 模拟校验和计算。
type mockChecksumer struct {
	checksums map[string]string // path -> checksum
}

func newMockChecksumer() *mockChecksumer {
	return &mockChecksumer{checksums: make(map[string]string)}
}

func (m *mockChecksumer) add(path, checksum string) {
	m.checksums[path] = checksum
}

func (m *mockChecksumer) mockChecksum() FileChecksumFunc {
	return func(path string) (string, error) {
		if c, ok := m.checksums[path]; ok {
			return c, nil
		}
		return "", os.ErrNotExist
	}
}

func TestResolveAction_FileNotExists(t *testing.T) {
	mfs := newMockFS()
	mc := newMockChecksumer()
	action := ResolveAction("/not/exists", "", "", mfs.mockStat(), mc.mockChecksum())
	if action != ActionDownload {
		t.Errorf("expected ActionDownload, got %v", action)
	}
}

func TestResolveAction_CompleteETagChecksumMatch(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	mc.add("/path/file.mp4", "abc123")

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionSkip {
		t.Errorf("expected ActionSkip, got %v", action)
	}
}

func TestResolveAction_CompleteETagMatchChecksumMismatch(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	mc.add("/path/file.mp4", "def456") // 文件实际校验和为 def456，但记录为 abc123

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionReDownload {
		t.Errorf("expected ActionReDownload (file corrupted), got %v", action)
	}
}

func TestResolveAction_CompleteETagMatchChecksumMissing(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	// 不添加 checksum，模拟文件无法计算校验和

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionResume {
		t.Errorf("expected ActionResume, got %v", action)
	}
}

func TestResolveAction_IncompleteFile(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 512*1024) // 不完整的文件
	mc := newMockChecksumer()

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionResume {
		t.Errorf("expected ActionResume, got %v", action)
	}
}

func TestResolveAction_CompleteETagChecksumReDownload(t *testing.T) {
	// 场景：文件完整，有 ETag 和 checksum 记录，但文件实际 checksum 与记录不一致（文件损坏）
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	mc.add("/path/file.mp4", "actualChecksum") // 本地文件实际校验和

	// prevChecksum 不一致 → 文件损坏 或 被篡改
	action := ResolveAction("/path/file.mp4", `"someETag"`, "recordedChecksum", mfs.mockStat(), mc.mockChecksum())
	if action != ActionReDownload {
		t.Errorf("expected ActionReDownload (checksum mismatch = corrupted), got %v", action)
	}
}

func TestResolveAction_OldDataNoETagChecksumMatch(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	mc.add("/path/file.mp4", "abc123")

	action := ResolveAction("/path/file.mp4", "", "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionSkip {
		t.Errorf("expected ActionSkip (old data, checksum matches), got %v", action)
	}
}

func TestResolveAction_OldDataNoETagNoChecksum(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()

	action := ResolveAction("/path/file.mp4", "", "", mfs.mockStat(), mc.mockChecksum())
	if action != ActionDownload {
		t.Errorf("expected ActionDownload, got %v", action)
	}
}

func TestResolveAction_FileZeroSize(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 0)
	mc := newMockChecksumer()

	action := ResolveAction("/path/file.mp4", "", "", mfs.mockStat(), mc.mockChecksum())
	if action != ActionDownload {
		t.Errorf("expected ActionDownload for zero-size file, got %v", action)
	}
}
