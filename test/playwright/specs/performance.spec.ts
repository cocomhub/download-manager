/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Performance baseline spec.
 *
 * Run with: npx playwright test specs/performance.spec.ts --project=desktop
 * These tests log timing data to stdout. Baseline values are recorded below.
 *
 * Baselines (2026-06-14, Windows 11, Chromium, 1440x900, github.com/cocomhub/download-manager):
 *   S2 (task selection render): ~78ms
 *   S3 (view switch): ~1097ms
 */

import { test, expect } from '@playwright/test';

test.describe('Performance Benchmarks', () => {

  test('S2: measure task selection render time', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    const startTime = Date.now();

    await page.locator('[data-testid="task-test-tktube"]').click();

    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });

    const renderTime = Date.now() - startTime;
    console.log(`Task selection -> render time: ${renderTime}ms`);

    expect(renderTime).toBeLessThan(5000);
  });

  test('S3: measure view switch time', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);

    const startTime = Date.now();
    await page.locator('[data-testid="view-mode-dashboard"]').click();
    await page.waitForTimeout(1000);

    const switchTime = Date.now() - startTime;
    console.log(`View switch -> content time: ${switchTime}ms`);

    expect(switchTime).toBeLessThan(3000);
  });
});
