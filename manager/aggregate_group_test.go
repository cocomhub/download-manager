package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

type mockTaskWithStore struct {
	id   string
	typ  string
	objs []*model.DownloadObject
}

func (m *mockTaskWithStore) ID() string                            { return m.id }
func (m *mockTaskWithStore) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (m *mockTaskWithStore) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (m *mockTaskWithStore) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return nil
}
func (m *mockTaskWithStore) Type() string                           { return m.typ }
func (m *mockTaskWithStore) Close() error                           { return nil }
func (m *mockTaskWithStore) GetAllObjects() []*model.DownloadObject { return m.objs }

func TestAggregateByContent_SelectRepresentativeAndSize(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "tktube"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	// Group A: one HQ and one C
	o1 := &model.DownloadObject{
		TaskID: "t1", URL: "u1",
		Metadata: map[string]string{"title": "【高画质】CLUB-100", "content_group": "CLUB-100", "date": "2024-01-01"},
		Extra:   map[string]any{},
	}
	o2 := &model.DownloadObject{
		TaskID: "t1", URL: "u2",
		Metadata: map[string]string{"title": "CLUB-100C", "content_group": "CLUB-100", "date": "2024-01-02"},
		Extra:   map[string]any{},
	}
	// Group B: single item no flags
	o3 := &model.DownloadObject{
		TaskID: "t1", URL: "u3",
		Metadata: map[string]string{"title": "ABP-456", "content_group": "ABP-456", "date": "2024-02-01"},
		Extra:   map[string]any{},
	}
	t1 := &mockTaskWithStore{
		id:   "t1",
		typ:  "tktube",
		objs: []*model.DownloadObject{o1, o2, o3},
	}
	m.tasks.Store("t1", t1)

	res, err := m.AggregateByContent(1, -1, "", "date_desc", "all", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs := res["objects"].([]*model.DownloadObject)
	if len(objs) != 2 {
		t.Fatalf("expect 2 groups, got %d", len(objs))
	}
	// Find group CLUB-100 representative should be o1 (has HQ)
	var repA *model.DownloadObject
	var repB *model.DownloadObject
	for _, o := range objs {
		if o.Metadata["content_group"] == "CLUB-100" {
			repA = o
		}
		if o.Metadata["content_group"] == "ABP-456" {
			repB = o
		}
	}
	if repA == nil || repA.URL != "u1" {
		t.Fatalf("expect CLUB-100 representative u1, got %+v", repA)
	}
	if repA.Extra["group_size"] != 2 {
		t.Fatalf("expect group_size 2 for CLUB-100, got %v", repA.Extra["group_size"])
	}
	if repB == nil || repB.URL != "u3" {
		t.Fatalf("expect ABP-456 representative u3, got %+v", repB)
	}
	if repB.Extra["group_size"] != 1 {
		t.Fatalf("expect group_size 1 for ABP-456, got %v", repB.Extra["group_size"])
	}
}

