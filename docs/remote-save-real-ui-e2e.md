# Remote-Save 真实 UI 全链路验收

> 目的：验证“浏览器 UI → 真实 download-manager → 真实 sproxy → 云端”的完整链路。
> 本文是手动/可自动化的验收清单；不依赖生产 sproxy，必须使用独立实例与唯一测试目录。

## 1. 前置条件

- 独立 sproxy：独立端口、独立 root、独立数据库/目录，不接受生产流量。
- 独立 download-manager：独立 `work_dir`、独立 Mongo 数据库（或 memory storage）。
- 环境变量（不写进仓库）：

```bash
export SPROXY_E2E_URL=https://127.0.0.1:18086
export SPROXY_E2E_AK=...
export SPROXY_E2E_SK=...
export SPROXY_E2E_SKID=...
```

- sproxy 侧确认：
  - `main` volume 可写；
  - 签名 AccessKey 有效；
  - `play_base_url` 指向可访问的 sproxy。

## 2. 测试任务

`url_list` 任务，`remote_save` 开启：

```yaml
tasks:
  - id: e2e-remote-save-ui
    type: url_list
    save_dir: <work_dir>/downloads/e2e
    storage: {type: memory}
    extra:
      urls:
        - http://127.0.0.1:<local-port>/ABC-001.mp4
      remote_save:
        enabled: true
        volume: main
        dir: e2e/remote-save-ui-<unix>
        object_dirs: true
        mirror_delete: true
```

本地提供测试文件；不要用真实业务文件。

## 3. 浏览器验收步骤

打开真实 download-manager UI：

1. 选择任务 `e2e-remote-save-ui`。
2. 等待对象变为 `completed`。
3. 点击“远程保存”。
4. 确认：
   - 弹窗显示任务 volume/dir；
   - “全局累计指标”明确标注为全局；
   - 对象状态、字节进度、文件数可见；
   - 未启用任务时按钮为灰色并提示。
5. 点击“立即对账”。
6. 等待对象状态变为 `synced`；期间弹窗应自动刷新（无需手动点刷新）。
7. 在 sproxy 侧预置同路径不同内容，制造 `checksum_mismatch`。
8. 再次对账，确认冲突出现在 UI。
9. 逐项验证至少两种动作：
   - `复用`：确认远端 hash 被接受；
   - `覆盖`：确认远端被替换、状态回到 `synced`。
10. 点击“播放”，确认返回内容与本地一致。
11. 修改任务目录策略制造迁移建议，点击“执行迁移”：
    - 确认提示“本批 100 条”与“剩余 N 条”；
    - 执行后查看“最近迁移审计”；
    - 导出 JSON 并核对 old/new path。
12. 删除对象，再次对账：
    - 确认远端文件被 mirror delete；
    - 确认 UI 不再展示该对象为 synced。
13. 停止 sproxy 后触发对账：
    - 确认 UI 显示明确错误；
    - 确认对象进入失败/重试；
    - 确认“下次重试”时间可见；
    - 恢复 sproxy 后等待自动重试或强制重试成功。

## 4. 自动化建议

当前 CI：

- Playwright：mock API，验证 UI 接线；
- `test/e2e`：真实 API + 真实 sproxy，但非浏览器。

建议新增一个不进入普通 CI 的 nightly job：

```bash
# 1. 启动独立 sproxy（唯一 root/端口）
# 2. 启动 download-manager（唯一 work_dir/Mongo）
# 3. 启动本地文件服务
# 4. Playwright 连真实 URL：
REMOTE_SAVE_REAL_UI_URL=http://127.0.0.1:<dm-port> \
REMOTE_SAVE_REAL_TASK_ID=e2e-remote-save-ui \
REMOTE_SAVE_REAL_ALLOW_WRITE=1 \
REMOTE_SAVE_REAL_EXPECT_SYNCED=1 \
  npx playwright test specs/remote-save-real.spec.ts

未设置环境变量时该用例自动 skip，不影响普通 CI。
```

自动化断言至少包含：

- 对象 completed；
- 对账后 synced；
- 冲突可见且可处理；
- 播放内容一致；
- 迁移批次/剩余/审计；
- mirror delete；
- sproxy 停止时 UI 有明确错误与重试时间。

## 5. 清理

- 删除 sproxy 测试目录/对象；
- 删除独立 `work_dir`；
- 删除测试 Mongo 数据库；
- 保留 `migration-audit.jsonl` 作为本次验收证据（如需归档）。
