// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import "errors"

// errTaskNotFound is a sentinel error returned when a task is not found
// in the manager's task registry. Callers should use errors.Is() to check.
var errTaskNotFound = errors.New("task not found")

// metaJSONSuffix is the file extension suffix for backup metadata files.
const metaJSONSuffix = ".meta.json"
