// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"os"
	"testing"
	"time"
)

// mockFileEntry 妯℃嫙鏂囦欢鏉＄洰
type mockFileEntry struct {
	size int64
}

// mockFileInfo 妯℃嫙 os.FileInfo
type mockFileInfo struct {
	size int64
}

func (m *mockFileInfo) Name() string       { return "" }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }

// mockFileSystem 妯℃嫙鏂囦欢绯荤粺鐘舵€?type mockFileSystem struct {
	files map[string]*mockFileEntry
}

func newMockFS() *mockFileSystem {
	return &mockFileSystem{files: make(map[string]*mockFileEntry)}
}

func (m *mockFileSystem) addFile(path string, size int64) {
	m.files[path] = &mockFileEntry{size: size}
}

// mockStat 杩斿洖 FileStatFunc銆?func (m *mockFileSystem) mockStat() FileStatFunc {
	return func(path string) (os.FileInfo, error) {
		e, ok := m.files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return &mockFileInfo{size: e.size}, nil
	}
}

// mockChecksumer 妯℃嫙鏍￠獙鍜岃绠椼€?type mockChecksumer struct {
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
	mc.add("/path/file.mp4", "def456") // 鏂囦欢瀹為檯鏍￠獙鍜屼负 def456锛屼絾璁板綍涓?abc123

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionReDownload {
		t.Errorf("expected ActionReDownload (file corrupted), got %v", action)
	}
}

func TestResolveAction_CompleteETagMatchChecksumMissing(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	// 涓嶆坊鍔?checksum锛屾ā鎷熸枃浠舵棤娉曡绠楁牎楠屽拰

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionResume {
		t.Errorf("expected ActionResume, got %v", action)
	}
}

func TestResolveAction_IncompleteFile(t *testing.T) {
	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 512*1024) // 涓嶅畬鏁寸殑鏂囦欢
	mc := newMockChecksumer()

	action := ResolveAction("/path/file.mp4", `"abc123"`, "abc123", mfs.mockStat(), mc.mockChecksum())
	if action != ActionResume {
		t.Errorf("expected ActionResume, got %v", action)
	}
}

func TestResolveAction_CompleteETagChecksumReDownload(t *testing.T) {
	// 鍦烘櫙锛氭枃浠跺畬鏁达紝鏈?ETag 鍜?checksum 璁板綍锛屼絾鏂囦欢瀹為檯 checksum 涓庤褰曚笉涓€鑷达紙鏂囦欢鎹熷潖锛?	mfs := newMockFS()
	mfs.addFile("/path/file.mp4", 1024*1024)
	mc := newMockChecksumer()
	mc.add("/path/file.mp4", "actualChecksum") // 鏈湴鏂囦欢瀹為檯鏍￠獙鍜?
	// prevChecksum 涓嶄竴鑷?鈫?鏂囦欢鎹熷潖 鎴?琚鏀?	action := ResolveAction("/path/file.mp4", `"someETag"`, "recordedChecksum", mfs.mockStat(), mc.mockChecksum())
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
