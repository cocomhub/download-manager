package task

import (
	"download-manager/core"
	"download-manager/model"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type SimpleTask struct {
	id      string
	urls    []string
	saveDir string
	objects []*model.DownloadObject
	mu      sync.Mutex
}

// Ensure SimpleTask implements core.Task
var _ core.Task = &SimpleTask{}

func NewSimpleTask(id string, urls []string, saveDir string) *SimpleTask {
	t := &SimpleTask{
		id:      id,
		urls:    urls,
		saveDir: saveDir,
		objects: make([]*model.DownloadObject, 0),
	}

	// Initialize objects
	usedNames := make(map[string]bool)
	for i, u := range urls {
		filename := filepath.Base(u)
		if filename == "." || filename == "/" {
			filename = fmt.Sprintf("file_%d.dat", i)
		}

		// Handle duplicates
		originalName := filename
		counter := 1
		for usedNames[filename] {
			ext := filepath.Ext(originalName)
			name := strings.TrimSuffix(originalName, ext)
			filename = fmt.Sprintf("%s_%d%s", name, counter, ext)
			counter++
		}
		usedNames[filename] = true
		
		obj := &model.DownloadObject{
			URL:      u,
			SavePath: filepath.Join(saveDir, filename),
			Status:   model.StatusPending,
		}
		t.objects = append(t.objects, obj)
	}
	return t
}

func (t *SimpleTask) ID() string {
	return t.id
}

func (t *SimpleTask) Type() string {
	return "simple_url_list"
}

func (t *SimpleTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var pending []*model.DownloadObject
	for _, obj := range t.objects {
		if obj.Status == model.StatusPending || obj.Status == model.StatusFailed {
			pending = append(pending, obj)
		}
	}
	return pending, nil
}

func (t *SimpleTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status
	if err != nil {
		fmt.Printf("[Task %s] Object %s failed: %v\n", t.id, obj.URL, err)
	} else {
		fmt.Printf("[Task %s] Object %s status updated to: %s\n", t.id, obj.URL, status)
	}
	return nil
}
