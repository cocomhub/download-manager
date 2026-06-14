/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Visual Regression', () => {

  test('V1: heading area matches snapshot', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Capture just the heading (stable content, no dynamic elements)
    const heading = page.locator('h1:has-text("Tasks")');
    await expect(heading).toHaveScreenshot('heading.png', {
      maxDiffPixels: 100,
    });
  });

  test('V2: task objects grid matches snapshot', async ({ page }) => {
    await page.goto('/');

    // Select test-tktube and wait for grid
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(2000);

    // Capture the grid area — mask the task-specific status indicator
    const main = page.locator('main');
    await expect(main).toHaveScreenshot('task-grid.png', {
      maxDiffPixels: 500,
    });
  });

  test('V3: task header area matches snapshot after selection', async ({ page }) => {
    await page.goto('/');

    // Select a task to get the full header
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(2000);

    // Capture the task header area (stable content)
    const header = page.locator('h2:has-text("test-tktube")');
    await expect(header).toHaveScreenshot('task-header.png', {
      maxDiffPixels: 100,
    });
  });
});
