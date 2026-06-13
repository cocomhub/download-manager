/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('UI-only Mode & Error Boundaries', () => {

  test('T14: UI renders correctly in full mode', async ({ page }) => {
    await page.goto('/');

    // Verify navigation elements
    await expect(page.locator('[data-testid="view-mode-downloads"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-aggregate"]') .or(page.locator('button:has-text("聚合")'))).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-dashboard"]')  .or(page.locator('button:has-text("仪表")'))).toBeVisible();

    // Sidebar visible
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });
});
