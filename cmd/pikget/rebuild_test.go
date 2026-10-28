// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocomhub/sproxy/pkg/volume/ext/pikpak"
)

// seedHybridFile 写一个「分片下载产物」测试文件：holes 指定的块清零；返回 (路径, 完整数据)。
func seedHybridFile(t *testing.T, chunkSize int64, nChunks int, holes ...int) (string, []byte) {
	t.Helper()
	payload := make([]byte, int64(nChunks)*chunkSize)
	for i := range payload {
		payload[i] = byte(i%251 + 1) // 非零，避免误触发零尾判定
	}
	data := make([]byte, len(payload))
	copy(data, payload)
	for _, idx := range holes {
		off := int64(idx) * chunkSize
		for i := off; i < off+chunkSize; i++ {
			data[i] = 0
		}
	}
	path := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	return path, payload
}

// seedManifest 写 manifest（模拟旧版本遗留的进度文件）。
func seedManifest(t *testing.T, path string, total, chunkSize int64, doneChunks int) {
	t.Helper()
	chunks := map[string]int64{}
	for i := range doneChunks {
		off := int64(i) * chunkSize
		chunks[fmt.Sprint(off)] = chunkSize
	}
	m := map[string]any{
		"total": total, "share_end": total / 2, "chunks": chunks,
		"source": map[string]any{"share_id": "abc123", "file_id": "f1", "hash": "deadbeef", "size": total},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path+".hybrid", data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// TestRunRebuild_OfflineScanDetectsZeroTailHole 离线扫描（本地路径，不联网）：
// 「块内尾部连续零」= 未写完的异常块 → 报告分片号并返回 exitFail；数据完整 → exitOK。
func TestRunRebuild_OfflineScanDetectsZeroTailHole(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	path, _ := seedHybridFile(t, chunk, 4, 2) // chunk2 为零洞
	seedManifest(t, path, 4*chunk, chunk, 4)  // manifest 谎报全部完成

	var out, errBuf bytes.Buffer
	code := runRebuild([]string{"--chunk-size", "1MiB", path}, &out, &errBuf)
	if code != exitFail {
		t.Fatalf("存在零尾洞应返回 exitFail(%d)，实际 %d；stdout=%q stderr=%q", exitFail, code, out.String(), errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "#2") {
		t.Errorf("报告应指出分片 #2: %q", got)
	}
	if !strings.Contains(got, "待重建") {
		t.Errorf("报告应含「待重建」: %q", got)
	}
	if !strings.Contains(got, "零尾") {
		t.Errorf("报告应说明零尾判定: %q", got)
	}
	// 离线扫描不得改动文件
	if fileSize(path) != 4*chunk {
		t.Errorf("离线扫描不得改动文件: size = %d", fileSize(path))
	}
	if _, err := os.Stat(path + ".hybrid"); err != nil {
		t.Errorf("离线扫描不得删除 manifest: %v", err)
	}
}

// TestRunRebuild_OfflineScanClean 数据完整 → 报告无需重建 + exitOK。
func TestRunRebuild_OfflineScanClean(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	path, _ := seedHybridFile(t, chunk, 3)
	seedManifest(t, path, 3*chunk, chunk, 3)

	var out, errBuf bytes.Buffer
	code := runRebuild([]string{"--chunk-size", "1MiB", path}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("数据完整应 exitOK，实际 %d；stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "无需重建") {
		t.Errorf("应提示无需重建: %q", out.String())
	}
}

// TestRunRebuild_OfflineScanWithoutManifest 无 manifest 也能离线扫描
// （total 取文件大小；块大小取 --chunk-size）——用户「进度文件异常/丢失」场景。
func TestRunRebuild_OfflineScanWithoutManifest(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	path, _ := seedHybridFile(t, chunk, 4, 1, 3) // 两块零洞

	var out, errBuf bytes.Buffer
	code := runRebuild([]string{"--chunk-size", "1MiB", path}, &out, &errBuf)
	if code != exitFail {
		t.Fatalf("应检出零洞（exitFail），实际 %d；stdout=%q", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "无 manifest") {
		t.Errorf("应提示无 manifest（total 取文件大小）: %q", got)
	}
	if !strings.Contains(got, "#1") || !strings.Contains(got, "#3") {
		t.Errorf("应列出 #1 与 #3: %q", got)
	}
}

// TestRunRebuild_UsageErrors 参数缺失/非法 → exitUsage。
func TestRunRebuild_UsageErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
	}{
		{"无参数", nil},
		{"非法 chunk-size", []string{"--chunk-size", "notasize", "x.mp4"}},
		{"非法 zero-run", []string{"--zero-run", "zzz", "x.mp4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			if code := runRebuild(c.args, &out, &errBuf); code != exitUsage {
				t.Fatalf("code = %d, want exitUsage(%d)", code, exitUsage)
			}
		})
	}
}

// TestRebuildChunkIndexLabel 分片编号/偏移展示（CLI 提示与 sproxy ChunkError 对齐）。
func TestRebuildChunkIndexLabel(t *testing.T) {
	t.Parallel()
	s := chunkSpanLabel(pikpak.ChunkSpan{Offset: 1006632960, Length: 33554432}, 33554432)
	if !strings.Contains(s, "#30") || !strings.Contains(s, "1006632960") {
		t.Errorf("label = %q, want 含 #30 与 offset", s)
	}
}

// TestPrintHybridFailureHint 失败后给出可执行恢复指引（含分片号/续传/rebuild 命令）。
func TestPrintHybridFailureHint(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	// 复刻用户实测错误形态：url.Error 包装 net/http TLS 握手超时（net.Error → 瞬断）
	netErr := &url.Error{Op: "Get", URL: "https://api-drive.mypikpak.com/drive/v1/files/X?usage=FETCH",
		Err: errors.New("net/http: TLS handshake timeout")}
	err := fmt.Errorf("pikget hybrid: %w", &pikpak.ChunkError{Offset: 1006632960, Length: 33554432, Err: netErr})
	printHybridFailureHint(&buf, err, "EXAMPLE-106.avi", 32<<20)
	got := buf.String()
	for _, want := range []string{"#30", "1006632960", "瞬断", "重跑", "pikget rebuild"} {
		if !strings.Contains(got, want) {
			t.Errorf("提示缺少 %q: %q", want, got)
		}
	}
}

// TestPrintHybridFailureHint_NonChunkError 非分片错误也要给续传提示（不误报分片）。
func TestPrintHybridFailureHint_NonChunkError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	printHybridFailureHint(&buf, errors.New("pikpak 登录失效"), "out.mp4", 32<<20)
	got := buf.String()
	if strings.Contains(got, "失败分片") {
		t.Errorf("非分片错误不应展示失败分片: %q", got)
	}
	if !strings.Contains(got, "重跑") {
		t.Errorf("应提示可续传: %q", got)
	}
}

// TestStageRebuildInputsMovesAbnormalNames 用户手工把失败产物改名成 *.error 标记后，
// --data/--manifest 能把它们归位到标准位置（重建才能续用已下数据 + 免重输 URL）。
func TestStageRebuildInputsMovesAbnormalNames(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	dir := t.TempDir()
	dest := filepath.Join(dir, "movie.mp4")
	dataPath := dest + ".hybrid.downloading.error"
	manifestPath := dest + ".hybrid.error"
	if err := os.WriteFile(dataPath, make([]byte, 2*chunk), 0o644); err != nil {
		t.Fatalf("seed data: %v", err)
	}
	m := map[string]any{
		"total": 2 * chunk, "chunks": map[string]int64{"0": chunk},
		"source": map[string]any{"share_id": "abc", "file_id": "f1", "hash": "h", "size": 2 * chunk,
			"url": "https://mypikpak.com/s/abc123/tok"},
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	if err := stageRebuildInputs(dest, dataPath, manifestPath); err != nil {
		t.Fatalf("stageRebuildInputs: %v", err)
	}
	if !fileExists(pikpak.WorkingPath(dest)) {
		t.Error("数据应归位到 *.hybrid.downloading")
	}
	if !fileExists(pikpak.ManifestPath(dest)) {
		t.Error("manifest 应归位到 *.hybrid")
	}
	if fileExists(dataPath) || fileExists(manifestPath) {
		t.Error("原异常命名文件应已移走（避免留下歧义副本）")
	}
	if url, ok := rebuildSourceURL(dest); !ok || url != "https://mypikpak.com/s/abc123/tok" {
		t.Errorf("归位后应可读到分享 URL: %q ok=%v", url, ok)
	}
}

// TestStageRebuildInputsRejectsMissing 指定的路径不存在 → 明确报错（不静默跳过）。
func TestStageRebuildInputsRejectsMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "movie.mp4")
	if err := stageRebuildInputs(dest, filepath.Join(dir, "nope.bin"), ""); err == nil {
		t.Fatal("--data 不存在应报错")
	}
	if err := stageRebuildInputs(dest, "", filepath.Join(dir, "nope.hybrid")); err == nil {
		t.Fatal("--manifest 不存在应报错")
	}
}

// TestRunRebuild_DataFlagsRequireRepair --data/--manifest 会移动文件，只读扫描模式下必须拒绝。
func TestRunRebuild_DataFlagsRequireRepair(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	path, _ := seedHybridFile(t, chunk, 2)
	var out, errBuf bytes.Buffer
	code := runRebuild([]string{"--data", path, path}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("code = %d, want exitUsage(%d)", code, exitUsage)
	}
	if !strings.Contains(errBuf.String(), "--repair") {
		t.Errorf("应提示需与 --repair 同用: %q", errBuf.String())
	}
}

// TestRebuildDestHonorsOutput --repair 本地文件模式下显式 -o 优先（否则用文件名推导最终名）。
func TestRebuildDestHonorsOutput(t *testing.T) {
	t.Parallel()
	const chunk = 1 << 20
	dir := t.TempDir()
	work := filepath.Join(dir, "movie.mp4.hybrid.downloading")
	if err := os.WriteFile(work, make([]byte, chunk), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 无 manifest → --repair 会因缺 URL 报错，但错误信息里的 dest 必须尊重 -o
	custom := filepath.Join(dir, "renamed.mp4")
	var out, errBuf bytes.Buffer
	code := runRebuild([]string{"--repair", "-o", custom, work}, &out, &errBuf)
	if code != exitFail {
		t.Fatalf("无 manifest URL 应 exitFail，实际 %d", code)
	}
	if !strings.Contains(errBuf.String(), custom) {
		t.Errorf("错误信息应使用 -o 指定的 dest %q: %q", custom, errBuf.String())
	}
}
