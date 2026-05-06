// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

func TestStorageExistenceMap_UsesStorageAndRuntime(t *testing.T) {
	storeAny, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("new memory storage: %v", err)
	}
	store := storeAny.(*storage.MemoryStorage)
	persisted := &model.DownloadObject{TaskID: "t1", URL: "persisted", Status: "completed"}
	if err := store.Update(persisted); err != nil {
		t.Fatalf("persisted update: %v", err)
	}
	runtime := []*model.DownloadObject{{TaskID: "t1", URL: "runtime", Status: "pending"}}
	got := storageExistenceMap(store, runtime, []string{"persisted", "runtime", "missing"})
	if !got["persisted"] || !got["runtime"] || got["missing"] {
		t.Fatalf("unexpected existence map: %+v", got)
	}
}

func TestPruneRuntimeObjects_KeepNonTerminalAndBoundTerminal(t *testing.T) {
	objects := make([]*model.DownloadObject, 0, 320)
	for i := range 40 {
		objects = append(objects, &model.DownloadObject{URL: fmt.Sprintf("terminal-%d", i), Status: "completed"})
	}
	for i := range 40 {
		objects = append(objects, &model.DownloadObject{URL: fmt.Sprintf("pending-%d", i), Status: "pending"})
	}
	got := pruneRuntimeObjects(objects)
	if len(got) != 72 {
		t.Fatalf("expected 72 hot objects, got %d", len(got))
	}
	terminalCount := 0
	pendingCount := 0
	for _, obj := range got {
		switch obj.Status {
		case "completed":
			terminalCount++
		case "pending":
			pendingCount++
		}
	}
	if terminalCount != runtimeTerminalObjectLimit {
		t.Fatalf("expected %d terminal objects, got %d", runtimeTerminalObjectLimit, terminalCount)
	}
	if pendingCount != 40 {
		t.Fatalf("expected all pending objects kept, got %d", pendingCount)
	}
}
