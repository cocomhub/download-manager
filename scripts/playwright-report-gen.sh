#!/usr/bin/env bash
# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

#  Playwright test summary report generator
#  Generates a markdown report from Playwright test-results.
#  Usage: bash scripts/playwright-report-gen.sh [test-results-dir]

set -euo pipefail

RESULTS_DIR="${1:-test/playwright/test-results}"
REPORT_FILE="test/playwright/playwright-report/summary.md"

mkdir -p "$(dirname "$REPORT_FILE")"

echo "# Playwright Test Report" > "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "Generated: $(date -u '+%Y-%m-%d %H:%M:%S UTC')" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Count passed/failed from JSON results if available
if [ -f "${RESULTS_DIR}/.last-run.json" ]; then
    total=$(jq -r '.stats.expected // 0' "${RESULTS_DIR}/.last-run.json" 2>/dev/null || echo "0")
    failed=$(jq -r '.stats.unexpected // 0' "${RESULTS_DIR}/.last-run.json" 2>/dev/null || echo "0")
    duration=$(jq -r '.stats.duration // 0' "${RESULTS_DIR}/.last-run.json" 2>/dev/null || echo "0")
    echo "## Summary" >> "$REPORT_FILE"
    echo "- **Total:** $total passed, $failed failed" >> "$REPORT_FILE"
    echo "- **Duration:** ${duration}ms" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# List failed tests
echo "## Failures" >> "$REPORT_FILE"
if [ -d "$RESULTS_DIR" ]; then
    find "$RESULTS_DIR" -maxdepth 1 -type d | while read -r dir; do
        name=$(basename "$dir")
        if [ "$name" != "test-results" ] && [ -f "$dir/error-context.md" ]; then
            error=$(head -5 "$dir/error-context.md" 2>/dev/null || echo "unknown error")
            echo "- **$name**" >> "$REPORT_FILE"
            echo "  \`\`\`" >> "$REPORT_FILE"
            echo "  $error" >> "$REPORT_FILE"
            echo "  \`\`\`" >> "$REPORT_FILE"
        fi
    done
else
    echo "No failures." >> "$REPORT_FILE"
fi

echo "" >> "$REPORT_FILE"
echo "## Projects" >> "$REPORT_FILE"
echo "- Desktop (Chromium)" >> "$REPORT_FILE"
echo "- Mobile (Chromium)" >> "$REPORT_FILE"
echo "- Firefox" >> "$REPORT_FILE"
echo "- WebKit" >> "$REPORT_FILE"

echo "Report written to $REPORT_FILE"
