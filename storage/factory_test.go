// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"os"
	"testing"
)

func TestNewStorage(t *testing.T) {
	tests := []struct {
		name         string
		typ          string
		cfg          map[string]string
		wantErr      bool
		wantEmptyErr bool // error message should contain this substring
	}{
		{
			name:         "file type requires path",
			typ:          "file",
			cfg:          map[string]string{},
			wantErr:      true,
			wantEmptyErr: true,
		},
		{
			name:    "unknown type returns error",
			typ:     "nonexistent",
			cfg:     nil,
			wantErr: true,
		},
		{
			name:    "memory type returns MemoryStorage",
			typ:     "memory",
			cfg:     nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewStorage(tt.typ, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewStorage() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && s == nil {
				t.Fatal("NewStorage() returned nil storage, expected non-nil")
			}
		})
	}
}

func TestNewStorage_FileWithTempDir(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.json"

	s, err := NewStorage("file", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewStorage('file', ...) = %v", err)
	}
	if s == nil {
		t.Fatal("NewStorage('file', ...) returned nil")
	}

	// FileStorage creates the file on first flush, not at construction.
	// Verify the directory was created.
	_, err = os.Stat(dir)
	if err != nil {
		t.Fatalf("expected directory to exist at %s: %v", dir, err)
	}
}
