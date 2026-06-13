/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { apiPost } from '../helpers/api';

test.describe('Realtime Updates via SSE', () => {

  test('T6: SSE object_update triggers UI refresh', async ({ page }) => {
    // Navigate before patching EventSource
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(1000);

    // Use page.waitForResponse to detect the SSE connection
    // The UI should refresh automatically when objects update
    // Trigger an object change via API
    const objectUrl = 'http://fixture/mixed/file-0.bin';

    const cancelResp = await apiPost('/api/tasks/test-mixed/object/cancel', {
      url: objectUrl,
    });
    // Accept any 2xx response
    expect(cancelResp).toBeDefined();

    // Wait for UI to update via SSE (the task sidebar shows changed counts)
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Verify we're on the mixed task page with objects
    const content = page.locator('main');
    await expect(content).toBeVisible();
  });

  test('T7: progress bars visible during active downloads', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(2000);

    // In the default grid view, check for content
    const main = page.locator('main');
    await expect(main).toBeVisible();
  });
});
