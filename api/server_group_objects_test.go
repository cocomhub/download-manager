package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

func TestGetGroupObjects_RequiresTaskScope(t *testing.T) {
	mgr := manager.NewManager(&config.Config{})
	srv := NewServer(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/groups/CLUB-100/objects", nil)
	rr := httptest.NewRecorder()

	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp["error"] != "missing_scope" {
		t.Fatalf("unexpected error code: %+v", resp)
	}
}
