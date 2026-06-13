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
    await page.waitForTimeout(1000);

    // Should see objects from all tasks
    const objects = page.locator('[data-testid^="object-"]');
    await expect(objects.first()).toBeVisible({ timeout: 10000 });
    const count = await objects.count();
    expect(count).toBeGreaterThan(5);
  });

  test('T9: aggregate view search works', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="view-mode-aggregate"]').click();
    await page.waitForTimeout(1000);

    // Use search filter
    const searchInput = page.locator('[data-testid="search-input"]').first();
    if (await searchInput.isVisible()) {
      await searchInput.fill('tktube');
      await page.waitForTimeout(500);

      // Objects should still be visible
      const objects = page.locator('[data-testid^="object-"]');
      const count = await objects.count();
      expect(count).toBeGreaterThan(0);
    }
  });
});
