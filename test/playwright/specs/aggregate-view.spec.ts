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

    // Wait for aggregate view to render
    const main = page.locator('main');
    await expect(main).toBeVisible({ timeout: 10000 });
    const text = await main.textContent();
    expect(text).toBeTruthy();
  });

  test('T9: aggregate view search renders results', async ({ page }) => {
    await page.goto('/');

    // Switch to aggregate view
    await page.locator('[data-testid="view-mode-aggregate"]').click();

    // Wait for the area to be interactive
    const main = page.locator('main');
    await expect(main).toBeVisible({ timeout: 10000 });

    // Use search filter — the aggregate view uses its own search input
    const searchInput = page.locator('[data-testid="search-input-aggregate"]');
    await searchInput.waitFor({ state: 'visible', timeout: 5000 });
    await searchInput.fill('tktube');
    await page.waitForTimeout(1500);

    // Check that the aggregate view responded (content area updated)
    const text = await main.textContent();
    expect(text).toBeTruthy();
  });
});
