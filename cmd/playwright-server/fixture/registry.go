package fixture

import (
	"github.com/cocomhub/download-manager/manager"
)

// fixtureFn is a function that populates a Manager with pre-defined tasks.
type fixtureFn func(*manager.Manager) error

// datasets maps fixture names to loader functions.
// Defined here and populated in datasets.go.
var datasets = map[string]fixtureFn{}
