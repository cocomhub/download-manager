package task

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"download-manager/core"
	"download-manager/model"
)

type SimpleTask struct {
	id      string
	urls    []string
	saveDir string
	objects []*model.DownloadObject
	store   core.Storage
	mu      sync.Mutex
}

// Ensure SimpleTask implements core.Task
var _ core.Task = &SimpleTask{}

func NewSimpleTask(id string, urls []string, saveDir string, store core.Storage) *SimpleTask {
	t := &SimpleTask{
		id:      id,
		urls:    urls,
		saveDir: saveDir,
		objects: make([]*model.DownloadObject, 0),
		store:   store,
	}

	// 1. Initialize potential objects from URLs (Source of Truth for "What to download")
	// 2. Check Storage for "What has been done" or "Current status"

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
			TaskID:   id,
			URL:      u,
			SavePath: filepath.Join(saveDir, filename),
			Status:   model.StatusPending,
		}

		// Check storage for this object
		if store != nil {
			// Try to get status by ID (using URL as ID for now)
			if storedObj, err := store.Get(u); err == nil && storedObj != nil {
				// Use stored status and metadata
				obj.Status = storedObj.Status
				obj.Metadata = storedObj.Metadata
				obj.Extra = storedObj.Extra

				// Fix "Zombie" downloading state
				// If we just started and storage says "downloading", it means it crashed.
				// Reset to pending.
				if obj.Status == model.StatusDownloading {
					fmt.Printf("[Task %s] Found zombie downloading state for %s, resetting to pending\n", id, u)
					obj.Status = model.StatusPending
					// We should probably sync this reset back to storage immediately or lazily
					// Let's sync immediately to be safe
					if err := store.Update(obj); err != nil {
						fmt.Printf("[Task %s] Failed to reset zombie state: %v\n", id, err)
					}
				}
			}
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

	// Print log
	if err != nil {
		fmt.Printf("[Task %s] Object %s failed: %v\n", t.id, obj.URL, err)
	} else {
		fmt.Printf("[Task %s] Object %s status updated to: %s\n", t.id, obj.URL, status)
	}

	// Update storage
	if t.store != nil {
		if storeErr := t.store.Update(obj); storeErr != nil {
			fmt.Printf("[Task %s] Failed to update storage: %v\n", t.id, storeErr)
		}
	}

	return nil
}

// New helper for API
func (t *SimpleTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Return copy to be safe? Or just slice.
	// Slice is reference to underlying array, but objects are pointers too.
	// For JSON serialization this should be fine as long as no concurrent modification happens during marshal.
	return t.objects
}
