// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

// maxSchedulerWeight is the maximum weight any single task can have in the
// weighted round-robin scheduler. Higher weight means more download slots.
const maxSchedulerWeight = 8

// recalcWeights recomputes scheduler weights for the given maxSchedulerWeight.
// Exported for testing; used by scheduler() and scheduler_test.
func (m *Manager) recalcWeights(weights map[string]int, maxSchedulerWeight int) map[string]int {
	m.tasks.Range(func(key, value any) bool {
		id := key.(string)
		w := 1
		w += max(0, len(m.getTaskQueue(id))/8)
		if v, ok := m.metrics.Load(id); ok {
			mt := v.(*taskMetrics)
			if mt.avgLatencyMs.Load() > 5000 {
				w -= 1
			}
			if mt.failures.Load() > 0 {
				w -= int(min(mt.failures.Load(), int64(2)))
			}
			if w < 1 {
				w = 1
			}
		}
		w = min(w, maxSchedulerWeight)
		weights[id] = w
		return true
	})
	return weights
}
