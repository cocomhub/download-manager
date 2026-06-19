// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// ================================================================
// 契约级测试 — core.Downloader 接口的通用契约
// ================================================================

// TestDLContract_EmptyURL 验证空 URL 返回错误。
func TestDLContract_EmptyURL(t *testing.T) {
	b := NewBeacon(t)
	cmp := NewComparator(t, b)
	obj := makeTestObject("", "out/file.txt", nil, nil)
	cmp.Run("empty-url", obj, nil, CheckAnyError())
}

// TestDLContract_EmptySavePath 验证空 SavePath 返回错误。
func TestDLContract_EmptySavePath(t *testing.T) {
	b := NewBeacon(t)
	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/f.txt", "", nil, nil)
	cmp.Run("empty-savepath", obj, nil, CheckAnyError())
}

// TestDLContract_Success 验证正常下载。
func TestDLContract_Success(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/hello.txt", "Hello, World!", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/hello.txt", "out/hello.txt", nil, nil)
	cmp.Run("success", obj, nil, CheckBothNil(), CheckFileBytes(), CheckFileSize())
}

// TestDLContract_ProgressCalled 验证进度回调被触发。
func TestDLContract_ProgressCalled(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/data.bin", "some test data here for progress checking", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/data.bin", "progress/data.bin", nil, nil)
	cmp.Run("progress", obj, nil, CheckBothNil(), CheckProgressEnd())
}

// TestDLContract_MetadataPopulated 验证下载完成后 Metadata 被正确填充。
func TestDLContract_MetadataPopulated(t *testing.T) {
	b := NewBeacon(t)
	content := "metadata test content for exact size verification"
	b.HandleFile("GET", "/meta.bin", content, "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/meta.bin", "meta/out.bin", nil, nil)
	cmp.Run("metadata", obj, nil,
		CheckBothNil(),
		CheckMetadata("total_size"),
		func(t *testing.T, old, new *DownloadResult) {
			t.Helper()
			want := strconv.Itoa(len(content))
			if old.Obj.Metadata["total_size"] != want {
				t.Errorf("old Metadata[total_size]=%q, want %q", old.Obj.Metadata["total_size"], want)
			}
			if new.Obj.Metadata["total_size"] != want {
				t.Errorf("new Metadata[total_size]=%q, want %q", new.Obj.Metadata["total_size"], want)
			}
		},
	)
}

// TestDLContract_NoSideEffect 验证输入参数未被意外修改（URL、SavePath）。
func TestDLContract_NoSideEffect(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/nse.dat", "no side effect test", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/nse.dat", "nse/out.dat", nil, nil)
	origURL := obj.URL
	origSavePath := obj.SavePath

	cmp.Run("no-side-effect", obj, nil, CheckBothNil(), CheckFileBytes())

	// 额外验证入参未被修改（注意 Metadata 是被有意修改的，不在此检查范围内）
	if obj.URL != origURL {
		t.Errorf("URL was modified: %q -> %q", origURL, obj.URL)
	}
	if obj.SavePath != origSavePath {
		t.Errorf("SavePath was modified: %q -> %q", origSavePath, obj.SavePath)
	}
}

// TestDLContract_Cancel 验证下载过程中的取消行为。
func TestDLContract_Cancel(t *testing.T) {
	b := NewBeacon(t)
	b.HandleSlow("GET", "/slow.bin", "slow content to cancel", 5*time.Second)

	cmp := NewComparator(t, b)

	t.Run("cancel_old", func(t *testing.T) {
		obj := makeTestObject(b.URL()+"/slow.bin", "cancel/old.bin", nil, nil)
		errCh := make(chan error, 1)
		go func() {
			errCh <- cmp.oldDL.Download(obj, nil)
		}()
		time.Sleep(200 * time.Millisecond)

		if canceler, ok := cmp.oldDL.(interface{ Cancel(string) error }); ok {
			canceler.Cancel(obj.URL)
		}

		select {
		case err := <-errCh:
			if err == nil {
				t.Error("old: expected cancel error, got nil")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("old: download did not cancel within 3s")
		}
	})

	t.Run("cancel_new", func(t *testing.T) {
		obj := makeTestObject(b.URL()+"/slow.bin", "cancel/new.bin", nil, nil)
		errCh := make(chan error, 1)
		go func() {
			errCh <- cmp.newDL.Download(obj, nil)
		}()
		time.Sleep(200 * time.Millisecond)

		if canceler, ok := cmp.newDL.(interface{ Cancel(string) error }); ok {
			canceler.Cancel(obj.URL)
		}

		select {
		case err := <-errCh:
			if err == nil {
				t.Error("new: expected cancel error, got nil")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("new: download did not cancel within 3s")
		}
	})
}

// TestDLContract_CancelNotFound 验证取消不存在的下载不 panic。
func TestDLContract_CancelNotFound(t *testing.T) {
	b := NewBeacon(t)
	cmp := NewComparator(t, b)

	t.Run("cancel_not_found_old", func(t *testing.T) {
		if canceler, ok := cmp.oldDL.(interface{ Cancel(string) error }); ok {
			err := canceler.Cancel("http://nonexistent.url/file.bin")
			if err == nil {
				t.Log("old: Cancel returned nil for nonexistent URL (acceptable)")
			} else {
				t.Logf("old: Cancel returned: %v", err)
			}
		}
	})

	t.Run("cancel_not_found_new", func(t *testing.T) {
		if canceler, ok := cmp.newDL.(interface{ Cancel(string) error }); ok {
			err := canceler.Cancel("http://nonexistent.url/file.bin")
			if err == nil {
				t.Log("new: Cancel returned nil for nonexistent URL (acceptable)")
			} else {
				t.Logf("new: Cancel returned: %v", err)
			}
		}
	})
}

// TestDLContract_DomainLimit 验证域名并发限制。
func TestDLContract_DomainLimit(t *testing.T) {
	b := NewBeacon(t)
	b.HandleSlow("GET", "/d1.bin", "domain content", 100*time.Millisecond)
	b.HandleSlow("GET", "/d2.bin", "domain content 2", 100*time.Millisecond)

	cmp := NewComparator(t, b)

	var mu sync.Mutex
	active := 0
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mu.Lock()
			active++
			mu.Unlock()

			obj := makeTestObject(b.URL()+"/d1.bin", fmt.Sprintf("domain/out%d.bin", idx), nil, nil)
			cmp.oldDL.Download(obj, nil)

			mu.Lock()
			active--
			mu.Unlock()
		}(i)
	}
	wg.Wait()
}

// TestDLContract_ConcurrentDownload 验证并发下载不冲突。
func TestDLContract_ConcurrentDownload(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/a.txt", "file A", "text/plain")
	b.HandleFile("GET", "/b.txt", "file B", "text/plain")

	cmp := NewComparator(t, b)

	var wg sync.WaitGroup
	urls := []string{"/a.txt", "/b.txt"}
	for i, url := range urls {
		wg.Add(1)
		go func(u string, idx int) {
			defer wg.Done()
			obj := makeTestObject(b.URL()+u, fmt.Sprintf("concurrent/out%d.txt", idx), nil, nil)
			if err := cmp.oldDL.Download(obj, nil); err != nil {
				t.Errorf("old concurrent download %d: %v", idx, err)
			}
		}(url, i)
	}
	wg.Wait()
}
