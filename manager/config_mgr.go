// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"github.com/cocomhub/download-manager/config"
)

type AuditInfo struct {
	Author  string
	Source  string
	Message string
}

func (m *Manager) GetConfig() *config.Config {
	return m.configSvc.GetConfig()
}

func (m *Manager) ListConfigBackups() ([]map[string]string, error) {
	return m.configSvc.ListConfigBackups()
}

func (m *Manager) DeleteConfigBackup(filename string) error {
	return m.configSvc.DeleteConfigBackup(filename)
}

func (m *Manager) RollbackConfig(filename string, audit *AuditInfo) error {
	cfg, err := m.configSvc.RollbackLoad(filename)
	if err != nil {
		return err
	}
	return m.UpdateConfig(cfg, audit)
}

func (m *Manager) DiffConfigFiles(left, right string) (map[string]any, error) {
	return m.configSvc.DiffConfigFiles(left, right)
}

func (m *Manager) DiffConfigFilesOpts(left, right string, ignoreWS, ignoreComments bool) (map[string]any, error) {
	return m.configSvc.DiffConfigFilesOpts(left, right, ignoreWS, ignoreComments)
}

func (m *Manager) AddConfigTag(filename, tag string) error {
	return m.configSvc.AddConfigTag(filename, tag)
}

func (m *Manager) AddConfigNote(filename, message, author string) error {
	return m.configSvc.AddConfigNote(filename, message, author)
}
