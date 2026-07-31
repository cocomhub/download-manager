// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"io"
	"strings"
	"sync"
	"testing"
)

func TestProgressReader(t *testing.T) {
	content := "Hello, World! This is a test for ProgressReader."
	reader := strings.NewReader(content)

	var lastProgress float64
	var lastDownloaded int64
	var lastTotal int64
	callCount := 0

	pr := NewProgressReader(reader, 0, int64(len(content)),
		func(progress float64, downloaded, total int64) {
			lastProgress = progress
			lastDownloaded = downloaded
			lastTotal = total
			callCount++
		},
	)

	// Read all data
	data, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != content {
		t.Errorf("expected content %q, got %q", content, string(data))
	}

	if callCount == 0 {
		t.Fatal("onProgress was never called")
	}

	if lastDownloaded != int64(len(content)) {
		t.Errorf("expected downloaded %d, got %d", len(content), lastDownloaded)
	}

	if lastTotal != int64(len(content)) {
		t.Errorf("expected total %d, got %d", len(content), lastTotal)
	}

	if lastProgress != 100.0 {
		t.Errorf("expected progress 100, got %f", lastProgress)
	}
}

func TestProgressReaderDone(t *testing.T) {
	var progress float64
	pr := NewProgressReader(strings.NewReader("data"), 0, 100,
		func(p float64, _, _ int64) {
			progress = p
		},
	)

	pr.Done()

	if progress != 100.0 {
		t.Errorf("Done() should set progress to 100, got %f", progress)
	}
}

func TestProgressReaderNilCallback(t *testing.T) {
	pr := NewProgressReader(strings.NewReader("data"), 0, 10, nil)

	data, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != "data" {
		t.Errorf("expected content 'data', got %q", string(data))
	}
}

func TestProgressReaderZeroTotal(t *testing.T) {
	var callCount int
	pr := NewProgressReader(strings.NewReader("test"), 0, 0,
		func(_ float64, _, _ int64) {
			callCount++
		},
	)

	_, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With total = 0, progress callback should not be called
	// because of the `if pr.total > 0` guard
	if callCount != 0 {
		t.Errorf("expected 0 callbacks with zero total, got %d", callCount)
	}
}

// TestProgressReader_WithInitialDownloaded 验证断点续传场景下，
// ProgressReader 从已下载字节数开始计算进度而非从 0 开始。
func TestProgressReader_WithInitialDownloaded(t *testing.T) {
	content := "hello world"
	initialDownloaded := int64(100) // simulate 100 bytes already downloaded
	totalSize := initialDownloaded + int64(len(content))

	var firstProgress float64
	var firstDownloaded int64
	callbackCalled := false

	pr := NewProgressReader(strings.NewReader(content), initialDownloaded, totalSize,
		func(progress float64, downloaded, total int64) {
			if !callbackCalled {
				firstProgress = progress
				firstDownloaded = downloaded
				callbackCalled = true
			}
		},
	)

	data, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected content %q, got %q", content, string(data))
	}

	if !callbackCalled {
		t.Fatal("onProgress was never called")
	}

	// First callback should report downloaded > initialDownloaded
	if firstDownloaded <= initialDownloaded {
		t.Errorf("expected downloaded > %d, got %d", initialDownloaded, firstDownloaded)
	}

	// Progress should reflect the initial offset, not 0%
	expectedMinProgress := float64(initialDownloaded) / float64(totalSize) * 100
	if firstProgress < expectedMinProgress {
		t.Errorf("expected progress >= %f%%, got %f%%", expectedMinProgress, firstProgress)
	}
}

// readerFunc 将函数适配为 io.Reader，用于测试中生成可被多个 goroutine 并发读取的数据源。
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// TestProgressReaderConcurrentSafety 验证 ProgressReader.downloaded 的并发安全性。
// 多个 goroutine 同时读取，所有 goroutine 完成后调用 Done()，验证最终进度和已下载字节数正确。
func TestProgressReaderConcurrentSafety(t *testing.T) {
	total := int64(100000)
	goroutines := 10

	// 共享的底层 reader，通过 mutex 保护内部位置指针，确保多个 goroutine 可并发调用
	var mu sync.Mutex
	var pos int64
	r := readerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		if pos >= total {
			return 0, io.EOF
		}
		n := len(p)
		if pos+int64(n) > total {
			n = int(total - pos)
		}
		pos += int64(n)
		return n, nil
	})

	var muProgress sync.Mutex
	var finalProgress float64
	var finalDownloaded int64

	pr := NewProgressReader(r, 0, total,
		func(progress float64, downloaded, _ int64) {
			muProgress.Lock()
			finalProgress = progress
			finalDownloaded = downloaded
			muProgress.Unlock()
		},
	)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			buf := make([]byte, 100)
			for {
				_, err := pr.Read(buf)
				if err != nil {
					break
				}
			}
		})
	}
	wg.Wait()

	pr.Done()

	muProgress.Lock()
	if finalProgress != 100.0 {
		t.Errorf("expected final progress 100%%, got %f%%", finalProgress)
	}
	if finalDownloaded != total {
		t.Errorf("expected final downloaded %d, got %d", total, finalDownloaded)
	}
	muProgress.Unlock()
}

func TestComposeProgressConcurrent(t *testing.T) {
	var mu sync.Mutex
	var callCount int
	cb := func(p float64, d, t int64) {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	composed := ComposeProgress(cb, cb)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			composed(50.0, 500, 1000)
		})
	}
	wg.Wait()

	mu.Lock()
	count := callCount
	mu.Unlock()
	if count != 200 {
		t.Errorf("expected progress callbacks to be invoked 200 times (100 calls × 2 callbacks), got %d", count)
	}
}
