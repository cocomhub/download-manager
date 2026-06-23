// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// TestEventBus_NoSubscriber verifies that publish does not block
// or panic when there are no active subscribers.
func TestEventBus_NoSubscriber(t *testing.T) {
	m := &Manager{
		subscribers: make(map[<-chan core.Event]chan core.Event),
	}

	// Should not block or panic with zero subscribers.
	m.publish(core.Event{Type: core.EventTaskUpdate, Payload: "hello"})
	m.publish(core.Event{Type: core.EventTaskListChange, Payload: nil})

	// Add subscriber then remove it.
	ch := m.Subscribe()
	m.Unsubscribe(ch)

	// Should still not block or panic after unsubscribe.
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: "data"})

	t.Log("publish with no subscribers completed without blocking or panic")
}
