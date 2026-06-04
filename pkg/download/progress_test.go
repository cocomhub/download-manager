package download

import (
	"io"
	"strings"
	"testing"
)

func TestProgressReader(t *testing.T) {
	content := "Hello, World! This is a test for ProgressReader."
	reader := strings.NewReader(content)

	var lastProgress float64
	var lastDownloaded int64
	var lastTotal int64
	callCount := 0

	pr := NewProgressReader(reader, int64(len(content)),
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
	pr := NewProgressReader(strings.NewReader("data"), 100,
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
	pr := NewProgressReader(strings.NewReader("data"), 10, nil)

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
	pr := NewProgressReader(strings.NewReader("test"), 0,
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