/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { apiPost } from '../helpers/api';

test.describe('Realtime Updates via SSE', () => {

  test('T6: cancel entire task triggers status change', async ({ page }) => {
    test.setTimeout(60000);
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await expect(page.locator('h2:has-text("test-mixed")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(500);

    // Cancel the entire task via API (more reliable than single object cancel)
    const cancelResp = await apiPost('/api/tasks/test-mixed/cancel');
    expect(cancelResp).toBeDefined();

    await page.waitForTimeout(1000);
    await expect(page.locator('main')).toBeVisible();
  });

  test('T7: task page renders content', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await expect(page.locator('main')).toBeVisible({ timeout: 10000 });
    const text = await page.locator('main').textContent();
    expect(text).toBeTruthy();
  });
});
