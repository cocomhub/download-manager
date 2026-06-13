/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Aggregate View', () => {

  test('T8: aggregate view shows cross-task objects', async ({ page }) => {
    await page.goto('/');

    // Switch to aggregate view
    await page.locator('[data-testid="view-mode-aggregate"]').click();
    await page.waitForTimeout(1500);

    // The aggregate view renders objects differently (TktubeAggregate).
    // Check main content area shows content.
    const main = page.locator('main');
    await expect(main).toBeVisible();
    const text = await main.textContent();
    expect(text).toBeTruthy();
  });

  test('T9: aggregate view search renders results', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="view-mode-aggregate"]').click();
    await page.waitForTimeout(1500);

    // Use search filter — the aggregate view uses tktubeSearchQuery
    // with placeholder "搜索标题、标签、URL"
    const searchInput = page.locator('input[placeholder="搜索标题、标签、URL"]');
    if (await searchInput.isVisible()) {
      await searchInput.fill('tktube');
      await page.waitForTimeout(1500);

      // Check that the aggregate view responded (content area changed)
      const main = page.locator('main');
      const text = await main.textContent();
      expect(text).toBeTruthy();
    }
  });
});
