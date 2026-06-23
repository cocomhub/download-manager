// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// mockTask implements core.Task with minimal stubs for aggregate tests.
type mockTask struct {
	id   string
	typ  string
	objs []*model.DownloadObject
}

func (m *mockTask) ID() string                            { return m.id }
func (m *mockTask) Type() string                          { return m.typ }
func (m *mockTask) Logger() *slog.Logger                  { return slog.Default() }
func (m *mockTask) Storage() core.Storage                 { return nil }
func (m *mockTask) SetDownloader(core.Downloader)         {}
func (m *mockTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (m *mockTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (m *mockTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return nil
}
func (m *mockTask) Concurrency() int                                               { return 1 }
func (m *mockTask) SetConcurrency(int) error                                       { return nil }
func (m *mockTask) RefreshInterval() int                                           { return 0 }
func (m *mockTask) SetRefreshInterval(int) error                                   { return nil }
func (m *mockTask) Start() error                                                   { return nil }
func (m *mockTask) ResolveObject(_ context.Context, _ *model.DownloadObject) error { return nil }
func (m *mockTask) Close() error                                                   { return nil }
func (m *mockTask) GetAllObjects(lock bool) []*model.DownloadObject                { return m.objs }
