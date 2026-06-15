#!/bin/sh
# git pre-commit hook: run Go standardization checks before commit
# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

echo "Running pre-commit checks..."

# 1. go fix
echo "  go fix ./..."
go fix ./... 2>/dev/null || true

# 2. go fmt
echo "  go fmt ./..."
go fmt ./...

# 3. addlicense (if available)
if command -v addlicense >/dev/null 2>&1; then
    echo "  addlicense..."
    addlicense -c "The Cocomhub Authors. All rights reserved." -s=only \
      -ignore ".claude/**" -ignore ".trae/**" -ignore ".cursor/**" .
elif command -v go >/dev/null 2>&1; then
    echo "  addlicense not found (install: go install github.com/google/addlicense@latest)"
fi

# 4. Build check (quick)
echo "  go build ./..."
go build ./...

echo "Pre-commit checks passed."
