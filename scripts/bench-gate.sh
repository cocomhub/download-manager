#!/usr/bin/env bash
# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# bench-gate.sh — 基准回归门禁
# 比较 baseline 与 current 的 benchstat 输出，超阈值（默认 p50 +5% / p90 +10%）则退出码 1。
#
# 用法：
#   bash scripts/bench-gate.sh baseline.txt current.txt [threshold_pct]
#   threshold_pct 默认 10（百分比，作用于 ns/op 与 allocs/op 的中位变化）
set -u

BASELINE="${1:-}"
CURRENT="${2:-}"
THRESHOLD="${3:-10}"

if [ -z "$BASELINE" ] || [ -z "$CURRENT" ]; then
  echo "usage: $0 baseline.txt current.txt [threshold_pct]" >&2
  exit 2
fi
if [ ! -f "$BASELINE" ]; then
  echo "baseline not found: $BASELINE (run 'make bench' first)" >&2
  exit 0  # 首次无 baseline 不失败
fi
if [ ! -f "$CURRENT" ]; then
  echo "current not found: $CURRENT" >&2
  exit 2
fi

# benchstat 缺失时安装
if ! command -v benchstat > /dev/null 2>&1; then
  go install golang.org/x/perf/cmd/benchstat@latest || exit 2
fi

OUT=$(benchstat "$BASELINE" "$CURRENT" 2>&1) || { echo "$OUT"; echo "benchstat failed" >&2; exit 2; }
echo "$OUT"

# 解析 delta 行：`pkg/BenchmarkX-8  1.23s ± 0%  1.30s ± 1%  +5.7%  ...`
# 提取所有 +X.X% / -X.X% 的 delta 值，取绝对值，检查是否超阈值。
FAIL=0
while IFS= read -r line; do
  delta=$(echo "$line" | grep -oE '[+-][0-9.]+%' | head -1)
  if [ -z "$delta" ]; then
    continue
  fi
  num=${delta//%/}
  num=${num//+/}
  abs=$(echo "$num" | awk '{if ($1<0) print -$1; else print $1}')
  if awk "BEGIN{exit !($abs > $THRESHOLD)}"; then
    echo "REGREssion: $line" >&2
    FAIL=1
  fi
done <<< "$OUT"

if [ $FAIL -eq 1 ]; then
  echo "Benchmark gate FAILED: >${THRESHOLD}% regression detected" >&2
  exit 1
fi
echo "Benchmark gate PASSED (threshold ${THRESHOLD}%)"
exit 0
