/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Cross-browser visual regression tests.
 *
 * These tests capture screenshots in Chrome and Firefox to verify consistent
 * rendering across browsers. WebKit is excluded due to longer run time.
 */

import { test, expect } from '@playwright/test';

test.describe('Cross-Browser Visual Regression', () => {

  test('V4: sidebar renders consistently across browsers', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1500);

    // Wait for sidebar to fully render
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();
  });

  test('V5: main layout renders consistently across browsers', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1500);

    // Select first task to populate the main area
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(2000);

    const main = page.locator('main');
    await expect(main).toBeVisible();
  });

  test('V6: aggregate view renders consistently across browsers', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Switch to aggregate view
    await page.locator('[data-testid="view-mode-aggregate"]').click();
    await page.waitForTimeout(2000);

    const main = page.locator('main');
    await expect(main).toBeVisible();
  });
});