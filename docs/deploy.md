# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# download-manager 部署指南

本文覆盖三种部署形态：**Docker Compose（推荐）**、**systemd 服务** 与 **裸二进制**，并给出环境变量清单与安全前提。

## 1. 安全前提（先读）

- **公网 / 不可信网络必须开启鉴权**，否则 API 与 `/files/` 裸暴露，任何人可读写下载内容。
- 鉴权由环境变量驱动，规格见 [阶段 3 设计规格](./superpowers/specs/2026-09-24-phase3-security-ops-design.md)：
  - `DM_AUTH_ENABLED`（`1`/`true`/`yes`，大小写不敏感）开启后，`auth.type` 未显式设置时默认 `basic`（用户名默认 `admin`）。
  - 本地二进制（无 `DM_AUTH_ENABLED`）保持 `auth.type=none`，行为不变。
- **Docker Compose 形态默认强制开启 basic 鉴权**，且 `DM_AUTH_PASSWORD` 为必填项——未设置时 `docker compose` 直接启动失败（fail-closed），杜绝无密码上线。
- systemd 形态不会自动开鉴权：请通过 `/etc/download-manager/env` 显式注入 `DM_AUTH_ENABLED=1` 等变量（见 §4.2）。

## 2. Docker Compose 部署

### 2.1 准备

- 主机需安装 Docker（含 compose 插件，`docker compose version` 可用）。
- 获取源码：`git clone <repo>` 后进入仓库根目录。

### 2.2 启动

```bash
export DM_AUTH_USERNAME=admin        # 可选，默认 admin
export DM_AUTH_PASSWORD='<强密码>'   # 必填，缺失时 compose 拒绝启动

docker compose up -d --build
```

- `--build` 会按根目录 `Dockerfile` 构建 `app` 镜像（golang:1.27-alpine 构建 → alpine:3.21 运行）。
- 首次启动会拉取 `mongo:7` 镜像并初始化数据卷。
- 应用端口映射为 `8080:8080`，访问 `http://<主机>:8080`，使用 `DM_AUTH_USERNAME` / `DM_AUTH_PASSWORD` 登录。

### 2.3 查看状态与日志

```bash
docker compose ps          # 两容器均应为 healthy
docker compose logs -f app # 应用日志
docker compose logs -f mongo
```

### 2.4 配置校验（语法检查）

```bash
docker compose config --quiet   # 退出 0 表示语法正确（注意：未设 DM_AUTH_PASSWORD 时按设计失败）
```

### 2.5 更新 / 停止 / 清理

```bash
docker compose up -d --build    # 重新构建并滚动更新
docker compose down             # 停止并移除容器（数据卷保留）
docker compose down -v          # 同时删除数据卷（数据不可恢复，谨慎）
```

## 3. systemd 部署

### 3.1 前置

- 将二进制安装到 `/usr/local/bin/download-manager`（构建：`make build`，产物 `build/bin/download-manager`）。
- 创建系统用户与数据目录：

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin download-manager || true
sudo mkdir -p /var/lib/download-manager /etc/download-manager
sudo chown -R download-manager:download-manager /var/lib/download-manager
```

### 3.2 配置文件

- `/etc/download-manager/config.yaml`：应用配置，注意：
  - `server.work_dir` 与下载根目录需落在 `/var/lib/download-manager` 下（数据持久化目录，属主 `download-manager`）。
  - `server.lock_file` 使用 `/var/lib/download-manager/download-manager.lock`（`ProtectSystem=strict` 下该目录由 `ReadWritePaths=/var/lib/download-manager` 显式放开为可写）。
  - `server.scraper_path` 指向二进制实际路径（如 `/usr/local/bin/scraper_get`），不使用则留空。
- `/etc/download-manager/env`：环境变量注入文件（`EnvironmentFile=`），见 §4.2。文件权限建议 `0600`（含密码）。

### 3.3 安装服务单元

```bash
sudo cp deploy/download-manager.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now download-manager
```

### 3.4 运维命令

```bash
sudo systemctl status download-manager   # 状态
sudo systemctl restart download-manager  # 重启（改配置后）
journalctl -u download-manager -f        # 日志
```

### 3.5 语法校验

```bash
systemd-analyze verify deploy/download-manager.service   # 退出 0 表示单元语法正确
```

## 4. 环境变量清单

应用启动时读取（配置优先级：命令行 > 环境变量 > 配置文件）。

### 4.1 鉴权（`DM_AUTH_*`）

| 变量 | 说明 | 默认 |
|------|------|------|
| `DM_AUTH_ENABLED` | 设为 `1`/`true`/`yes`（大小写不敏感）启用鉴权；启用且 `auth.type` 未设置时默认 `basic` | 空（不启用） |
| `DM_AUTH_TYPE` | 鉴权类型：`basic` / `token` / `none` | `basic`（`DM_AUTH_ENABLED=1` 时） |
| `DM_AUTH_USERNAME` | basic 用户名 | `admin` |
| `DM_AUTH_PASSWORD` | basic 密码（**必填**，Docker Compose 形态强制） | 空 |
| `DM_AUTH_TOKEN` | token 鉴权令牌（`DM_AUTH_TYPE=token` 时使用） | 空 |
| `DM_AUTH_TOKEN_EXPIRES` | token 过期时间，RFC3339 格式（如 `2027-01-01T00:00:00Z`），空 = 永不过期 | 空 |

### 4.2 systemd 形态的 `/etc/download-manager/env` 示例

```bash
DM_AUTH_ENABLED=1
DM_AUTH_TYPE=basic
DM_AUTH_USERNAME=admin
DM_AUTH_PASSWORD=<强密码>
```

### 4.3 运行与端口（其他常用变量）

| 变量 | 说明 | 默认 |
|------|------|------|
| `DM_RUN_MODE` | 运行模式 `full` / `ui`（`ui` 仅管理界面，写操作被拦截） | `full` |
| `DM_UI_ONLY` | 旧版开关：`1`/`true`/`yes` 等价 `ui` 模式（仅当 `DM_RUN_MODE` 未设置时生效） | — |
| `DM_HTTP_PORT` | HTTP 端口 | `8080` |

## 5. 数据持久化说明

| 数据 | Compose 形态 | systemd 形态 |
|------|--------------|--------------|
| 下载文件 | 命名卷 `dm-data`，挂载到容器内 `/data`（`server.work_dir` 与下载根目录应配置在该目录下，如 `/data/downloads`） | `/var/lib/download-manager` 目录 |
| MongoDB 数据 | 命名卷 `mongo-data`，挂载到 `/data/db` | 由外部 MongoDB 实例负责持久化 |

- Compose 数据卷由 Docker 管理，`docker compose down` 不删除卷；仅 `docker compose down -v` 才删除（数据不可恢复）。
- 备份建议：直接备份卷目录（`dm-data`、`mongo-data`），或对 MongoDB 执行 `mongodump`。

## 6. 裸二进制部署（本机 / 内网）

```bash
make build
./build/bin/download-manager --config config.yaml
```

本形态默认 `auth.type=none`；若暴露到公网，务必参照 §4.1 设置 `DM_AUTH_ENABLED=1` 等变量后再启动。
