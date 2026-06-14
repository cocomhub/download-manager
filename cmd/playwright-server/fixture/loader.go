// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"fmt"

	"github.com/cocomhub/download-manager/manager"
)

// LoadFixture loads a named dataset into the manager before Start().
// Delegates to datasets.go where all fixture definitions live.
func LoadFixture(mgr *manager.Manager, name string) error {
	if fn, ok := datasets[name]; ok {
		return fn(mgr)
	}
	return fmt.Errorf("unknown fixture: %s", name)
}
