/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('UI-only Mode & Error Boundaries', () => {

  // TODO: 完整 T14 场景需要启动第二个 server 实例（UI-only 模式）
  // 当前在 full mode 下验证基本导航功能
  // 后续在 playwright.config.ts 中添加第二个 project（ui-only）覆盖：
  //   - 所有操作按钮禁用/灰化
  //   - 写 API 返回 405
  //   - 提示写禁用
  test('T14: UI renders correctly in full mode', async ({ page }) => {
    await page.goto('/');

    // Verify navigation elements
    await expect(page.locator('[data-testid="view-mode-downloads"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-aggregate"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-dashboard"]')).toBeVisible();

    // Sidebar visible
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });
});
