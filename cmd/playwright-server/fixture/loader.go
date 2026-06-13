// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"github.com/cocomhub/download-manager/manager"
)

// LoadFixture loads a named dataset into the manager before Start().
// Available names: "full" (4 tasks, ~41 objects total).
func LoadFixture(mgr *manager.Manager, name string) error {
	switch name {
	case "full":
		return loadFull(mgr)
	default:
		return nil
	}
}
