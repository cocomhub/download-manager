// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestEventBus_SubscribeAndReceive verifies basic pub/sub works.
func TestEventBus_SubscribeAndReceive(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	event := core.Event{Type: core.EventTaskUpdate, Payload: "hello"}
	m.publish(event)

	select {
	case got := <-ch:
		if got.Type != core.EventTaskUpdate {
			t.Fatalf("expected EventTaskUpdate, got %v", got.Type)
		}
		if got.Payload != "hello" {
			t.Fatalf("expected payload 'hello', got %v", got.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestEventBus_MultipleSubscribers verifies all subscribers receive events.
func TestEventBus_MultipleSubscribers(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	ch1 := m.Subscribe()
	defer m.Unsubscribe(ch1)
	ch2 := m.Subscribe()
	defer m.Unsubscribe(ch2)

	m.publish(core.Event{Type: core.EventTaskListChange})

	for i, ch := range []<-chan core.Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Type != core.EventTaskListChange {
				t.Fatalf("subscriber %d: expected EventTaskListChange, got %v", i+1, got.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout waiting for event", i+1)
		}
	}
}

// TestEventBus_SlowConsumerDrop verifies that a slow subscriber gets dropped events.
func TestEventBus_SlowConsumerDrop(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	// Subscribe but never read — buffer is 100, so publishing >100 should trigger drops
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	for range 150 {
		m.publish(core.Event{Type: core.EventTaskUpdate})
	}

	// Channel should have exactly 100 events (buffer full, rest dropped)
	received := 0
	for {
		select {
		case <-ch:
			received++
		default:
			// No more events
			if received != 100 {
				t.Fatalf("expected 100 buffered events (buffer size), got %d", received)
			}
			return
		}
	}
}

// TestEventBus_Unsubscribe verifies unsubscribed channel receives no events and is closed.
func TestEventBus_Unsubscribe(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	ch := m.Subscribe()
	m.Unsubscribe(ch)

	// Channel should be closed after unsubscribe
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	// Publishing should not panic
	m.publish(core.Event{Type: core.EventTaskUpdate})
}

// TestEventBus_UnsubscribeNonExistent verifies unsubscribing a non-existent channel is safe.
func TestEventBus_UnsubscribeNonExistent(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	ch := make(chan core.Event, 1)
	// Should not panic
	m.Unsubscribe(ch)
}

// TestBroadcastProgress verifies broadcastProgress sends ProgressBatch with changed items.
func TestBroadcastProgress(t *testing.T) {
	m := &Manager{
		subscribers:  make(map[<-chan core.Event]chan core.Event),
		lastProgress: sync.Map{},
	}

	// Seed downloading objects
	obj1 := &model.DownloadObject{
		TaskID:   "task1",
		URL:      "http://example.com/file1",
		Metadata: map[string]string{"title": "File One"},
	}
	obj1.SetProgress(50)
	obj1.SetStatus(model.StatusDownloading)
	m.downloadingObj.Store(obj1.URL, obj1)

	obj2 := &model.DownloadObject{
		TaskID: "task1",
		URL:    "http://example.com/file2",
	}
	obj2.SetProgress(20)
	obj2.SetStatus(model.StatusDownloading)
	m.downloadingObj.Store(obj2.URL, obj2)

	// Subscribe
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	m.broadcastProgress()

	select {
	case got := <-ch:
		if got.Type != core.EventProgressBatch {
			t.Fatalf("expected EventProgressBatch, got %v", got.Type)
		}
		batch, ok := got.Payload.(*ProgressBatch)
		if !ok {
			t.Fatalf("expected *ProgressBatch payload, got %T", got.Payload)
		}
		if len(batch.Updates) != 2 {
			t.Fatalf("expected 2 progress updates, got %d", len(batch.Updates))
		}
		// Verify items — order depends on sync.Map iteration
		if len(batch.Updates) != 2 {
			t.Fatalf("expected 2 progress updates, got %d", len(batch.Updates))
		}
		// Both expected URLs must be present
		urls := map[string]bool{batch.Updates[0].URL: true, batch.Updates[1].URL: true}
		if !urls[obj1.URL] {
			t.Fatalf("expected progress update for %q, got %v", obj1.URL, batch.Updates)
		}
		if !urls[obj2.URL] {
			t.Fatalf("expected progress update for %q, got %v", obj2.URL, batch.Updates)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for progress batch")
	}
}

// TestBroadcastProgress_UnchangedProgressSkipped verifies objects with unchanged
// progress are filtered out.
func TestBroadcastProgress_UnchangedProgressSkipped(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	obj := &model.DownloadObject{
		TaskID: "task1",
		URL:    "http://example.com/file1",
	}
	obj.SetProgress(50)
	obj.SetStatus(model.StatusDownloading)
	m.downloadingObj.Store(obj.URL, obj)

	// Pre-set lastProgress to match current progress — simulates no change
	m.lastProgress.Store(obj.URL, 50)

	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	m.broadcastProgress()

	select {
	case <-ch:
		t.Fatal("expected no event when progress hasn't changed")
	case <-time.After(200 * time.Millisecond):
		// Expected: no event because no progress changed
	}
}

// TestBroadcastProgress_NoDownloadingObjects verifies no event when nothing is downloading.
func TestBroadcastProgress_NoDownloadingObjects(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	m.broadcastProgress()

	select {
	case <-ch:
		t.Fatal("expected no event with empty downloadingObj")
	case <-time.After(200 * time.Millisecond):
		// Expected
	}
}

// TestBroadcastTaskUpdate verifies BroadcastTaskUpdate publishes a task summary event.
func TestBroadcastTaskUpdate(t *testing.T) {
	m := &Manager{
		subscribers:     make(map[<-chan core.Event]chan core.Event),
		activeDownloads: make(map[string]int),
		metrics:         sync.Map{},
	}

	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	// Since no task is registered, BroadcastTaskUpdate should be a no-op
	m.BroadcastTaskUpdate("nonexistent")

	select {
	case <-ch:
		t.Fatal("expected no event for nonexistent task")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestBroadcastTaskUpdate_WithTask tests BroadcastTaskUpdate with a registered task.
func TestBroadcastTaskUpdate_WithTask(t *testing.T) {
	dl := mockdl.New(mockdl.ModeAlwaysSuccess)
	mgr, _ := newMockManager(t, "test-task", 3, dl)
	_ = startManager(t, mgr)

	task := waitForTask(t, mgr, "test-task")

	ch := mgr.Subscribe()
	defer mgr.Unsubscribe(ch)

	mgr.BroadcastTaskUpdate(task.ID())

	select {
	case got := <-ch:
		if got.Type != core.EventTaskUpdate {
			t.Fatalf("expected EventTaskUpdate, got %v", got.Type)
		}
		summary, ok := got.Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any payload, got %T", got.Payload)
		}
		if summary["id"] != task.ID() {
			t.Fatalf("expected task id %q, got %q", task.ID(), summary["id"])
		}
		if summary["type"] != "mock" {
			t.Fatalf("expected task type 'mock', got %q", summary["type"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for task update")
	}
}

// TestConcurrentPublish verifies concurrent publishing is safe.
func TestConcurrentPublish(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	subCount := 5
	chs := make([]<-chan core.Event, subCount)
	for i := range chs {
		chs[i] = m.Subscribe()
		defer m.Unsubscribe(chs[i])
	}

	var done atomic.Int64
	concurrent := 10
	for range concurrent {
		go func() {
			m.publish(core.Event{Type: core.EventTaskUpdate})
			done.Add(1)
		}()
	}

	// Wait for all goroutines to complete
	for done.Load() < int64(concurrent) {
		time.Sleep(10 * time.Millisecond)
	}

	// Each subscriber should have exactly 'concurrent' events
	for _, ch := range chs {
		count := 0
		for {
			select {
			case <-ch:
				count++
			default:
				if count != concurrent {
					t.Fatalf("expected %d events, got %d", concurrent, count)
				}
				goto next
			}
		}
	next:
	}
}
