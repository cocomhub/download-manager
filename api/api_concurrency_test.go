// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_ConcurrentRequests verifies API layer correctness under concurrent requests.
func TestAPI_ConcurrentRequests(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "conc-test", 10, true)
	r := srv.Router()
	_ = startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for manager to load tasks")

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
