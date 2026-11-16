/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * 真实 UI 冒烟：连接已启动的 download-manager 真实服务。
 * 默认只读；设置 REMOTE_SAVE_REAL_ALLOW_WRITE=1 才触发对账。
 *
 * 必填：
 *   REMOTE_SAVE_REAL_UI_URL=http://127.0.0.1:<dm-port>
 *   REMOTE_SAVE_REAL_TASK_ID=<已启用 remote_save 的测试任务>
 * 可选：
 *   REMOTE_SAVE_REAL_EXPECT_SYNCED=1  等待至少一个对象“已同步”
 */

import { test, expect } from '@playwright/test';

const baseURL = process.env.REMOTE_SAVE_REAL_UI_URL || '';
const taskID = process.env.REMOTE_SAVE_REAL_TASK_ID || '';
const allowWrite = process.env.REMOTE_SAVE_REAL_ALLOW_WRITE === '1';
const expectSynced = process.env.REMOTE_SAVE_REAL_EXPECT_SYNCED === '1';

test.describe('Remote Save Real UI', () => {
  test.skip(!baseURL || !taskID, '需要 REMOTE_SAVE_REAL_UI_URL + REMOTE_SAVE_REAL_TASK_ID');

  test('opens real remote-save panel', async ({ page }) => {
    await page.goto(baseURL, { waitUntil: 'domcontentloaded', timeout: 30000 });

    const task = page.locator('[data-testid="task-' + taskID + '"]');
    await expect(task).toBeVisible({ timeout: 30000 });
    await task.click();

    const button = page.locator('[data-testid="btn-remote-save"]');
    await expect(button).toBeVisible({ timeout: 15000 });
    await expect(button).not.toContainText('未启用');
    await button.click();

    const modal = page.locator('[data-testid="remote-save-modal"]');
    await expect(modal).toBeVisible({ timeout: 15000 });
    await expect(modal).not.toContainText('该任务未启用 remote_save');
    await expect(page.locator('[data-testid="remote-save-objects"]')).toBeVisible({ timeout: 30000 });

    if (allowWrite) {
      await page.locator('[data-testid="remote-save-trigger"]').click();
      if (expectSynced) {
        await expect.poll(async () => {
          return page.locator('[data-testid="remote-save-object"]').filter({ hasText: '已同步' }).count();
        }, { timeout: 180000, intervals: [5000] }).toBeGreaterThan(0);
      }
    }
  });
});
