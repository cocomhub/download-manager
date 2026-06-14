/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Visual Regression', () => {

  test('V1: sidebar matches snapshot', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    // Capture sidebar region
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toHaveScreenshot('sidebar.png', {
      maxDiffPixels: 100,
    });
  });

  test('V2: task objects grid matches snapshot', async ({ page }) => {
    await page.goto('/');

    // Select test-tktube and wait for grid
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1500);

    // Capture the grid area
    const main = page.locator('main');
    await expect(main).toHaveScreenshot('task-grid.png', {
      maxDiffPixels: 200,
    });
  });

  test('V3: dashboard matches snapshot', async ({ page }) => {
    await page.goto('/');

    // Switch to dashboard
    await page.locator('[data-testid="view-mode-dashboard"]').click();
    await page.waitForTimeout(1500);

    const main = page.locator('main');
    await expect(main).toHaveScreenshot('dashboard.png', {
      maxDiffPixels: 200,
    });
  });
});
