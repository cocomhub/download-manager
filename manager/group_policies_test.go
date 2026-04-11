package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

type memStore struct {
	m map[string]*model.DownloadObject
}

func (s *memStore) Get(id string) (*model.DownloadObject, error) { return s.m[id], nil }
func (s *memStore) Update(obj *model.DownloadObject) error {
	if s.m == nil {
		s.m = make(map[string]*model.DownloadObject)
	}
	s.m[obj.URL] = obj
	return nil
}
func (s *memStore) Delete(id string) error { delete(s.m, id); return nil }
func (s *memStore) Search(filter any) ([]*model.DownloadObject, error) {
	var list []*model.DownloadObject
	for _, o := range s.m {
		list = append(list, o)
	}
	return list, nil
}

type fakeTktTask struct {
	id   string
	objs []*model.DownloadObject
	st   core.Storage
}

func (f *fakeTktTask) ID() string                            { return f.id }
func (f *fakeTktTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (f *fakeTktTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (f *fakeTktTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	obj.Status = status
	return f.st.Update(obj)
}
func (f *fakeTktTask) Type() string                           { return "tktube" }
func (f *fakeTktTask) Close() error                           { return nil }
func (f *fakeTktTask) GetAllObjects() []*model.DownloadObject { return f.objs }
func (f *fakeTktTask) GetStorage() core.Storage               { return f.st }

func TestApplyGroupPriorityPolicies_CancelLowerPriority(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}
	// Canonical: hasHQ
	o1 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	// Lower priority: hasC only
	o2 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	_ = store.Update(o1)
	_ = store.Update(o2)
	task := &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{o1, o2}}

	m.applyGroupPriorityPolicies(task, o1)

	if o2.Status != dlcore.StatusCancelled {
		t.Fatalf("expect o2 cancelled, got %s", o2.Status)
	}
	if o2.Extra["redirect_url"] != o1.URL {
		t.Fatalf("expect redirect_url=%s, got %v", o1.URL, o2.Extra["redirect_url"])
	}
}

