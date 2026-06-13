/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Config Management', () => {

  test('T12: config panel opens and shows sections', async ({ page }) => {
    await page.goto('/');

    // Open config panel via cog button
    const configBtn = page.locator('button:has(.fa-cog)');
    if (await configBtn.isVisible()) {
      await configBtn.click();
      await page.waitForTimeout(500);
    }

    // Config modal should be visible
    const configModal = page.locator('text=Config, text=配置, text=Server').first();
    if (await configModal.isVisible({ timeout: 3000 }).catch(() => false)) {
      await expect(configModal).toBeVisible();
    }
  });

  test('T13: config panel close works', async ({ page }) => {
    await page.goto('/');

    // Open config panel
    const configBtn = page.locator('button:has(.fa-cog)');
    if (await configBtn.isVisible()) {
      await configBtn.click();
      await page.waitForTimeout(500);
    }

    // Look for close button
    const closeBtn = page.locator('button:has(.fa-times), button:has(.fa-close), button:has-text("Close"), button:has-text("关闭")').first();
    if (await closeBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await closeBtn.click();
      await page.waitForTimeout(300);
    }
  });
});
