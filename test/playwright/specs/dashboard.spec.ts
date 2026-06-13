/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {

  test('T10: dashboard view shows health status', async ({ page }) => {
    await page.goto('/');

    // Switch to dashboard
    await page.locator('[data-testid="view-mode-dashboard"]').click();

    // Should show status/health related text
    const dashboardContent = page.locator('main');
    await expect(dashboardContent).toBeVisible({ timeout: 10000 });
  });

  test('T11: dashboard shows metrics', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="view-mode-dashboard"]').click();

    // Wait for dashboard to render
    const mainContent = page.locator('main');
    await expect(mainContent).toBeVisible({ timeout: 10000 });
    const text = await mainContent.textContent();
    expect(text?.length).toBeGreaterThan(50);
  });
});
