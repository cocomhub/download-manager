#!/usr/bin/env bash
# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# 起 mongo 容器 → 跑 storage 集成测试（-tags=integration）→ 清理容器。
# 无 docker 时提示（集成用例将回落到 testcontainers，两者都不可用时 t.Skip，不失败）。
#
# 行为（对齐 sproxy 的 scripts/test-vault.sh）：
#   - 若已有健康的本脚本容器 → 直接复用，不重启；
#   - 否则起 docker 容器（镜像钉版本：mongo:8，可用 MONGO_IMAGE 覆写）；
#   - 就绪探测用容器内 mongosh ping（30s 超时后明确失败）；
#   - 通过 MONGO_TEST_URI 让集成用例复用该实例（不起 testcontainers）。
#
# 用法：make test-mongo  （或直接 bash scripts/test-mongo.sh）
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v docker >/dev/null 2>&1; then
  echo "docker 不可用，跳过 mongo 集成测试（集成用例将回落到 testcontainers / t.Skip）" >&2
  exit 0
fi

name="dm-mongo-test"
MONGO_IMAGE="${MONGO_IMAGE:-mongo:8}"
container_started=0

# mongo_ready：容器内 mongosh ping（就绪即返回 0）。
mongo_ready() {
  docker exec "$name" mongosh --quiet --eval 'db.runCommand({ping:1}).ok' >/dev/null 2>&1
}

if mongo_ready; then
  echo "检测到已有健康 mongo 容器（$name）——直接复用"
else
  echo "启动 mongo 容器（$MONGO_IMAGE, :27017）..."
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run -d --rm --name "$name" -p 27017:27017 "$MONGO_IMAGE" >/dev/null
  container_started=1
  cleanup() {
    if [[ $container_started -eq 1 ]]; then
      docker rm -f "$name" >/dev/null 2>&1 || true
    fi
  }
  trap cleanup EXIT

  ready=0
  for _i in $(seq 1 30); do
    if mongo_ready; then
      ready=1
      break
    fi
    sleep 1
  done
  if [[ $ready -ne 1 ]]; then
    echo "错误：mongo 容器 30s 内未就绪（ping 未成功）" >&2
    exit 1
  fi
fi

echo "mongo 就绪，运行 storage 集成测试..."
MONGO_TEST_URI="${MONGO_TEST_URI:-mongodb://127.0.0.1:27017}" \
  go test -tags=integration -race -count=1 -timeout=300s ./storage/...
