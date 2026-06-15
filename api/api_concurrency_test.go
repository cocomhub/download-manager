// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestAPI_ConcurrentRequests verifies API layer correctness under concurrent requests.
func TestAPI_ConcurrentRequests(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "conc-test", 10, true)
	r := srv.Router()
	_ = startAPIManager(t, srv)
	time.Sleep(200 * time.Millisecond) // wait for loadTasks in manager goroutine

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/tasks", nil)
			r.ServeHTTP(rr, req)
			if rr.Code != 200 {
				t.Errorf("GET /api/tasks returned %d", rr.Code)
			}
		})
	}
	wg.Wait()
}
