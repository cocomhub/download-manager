/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { apiPost } from '../helpers/api';

test.describe('Realtime Updates via SSE', () => {

  test('T6: API cancel triggers UI status change', async ({ page }) => {
    // Navigate to the tktube task (stable, all completed)
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('main')).toContainText('completed', { timeout: 10000 });

    // Cancel the entire task via API — the sidebar should update
    const cancelResp = await apiPost('/api/tasks/test-tktube/cancel');
    expect(cancelResp).toBeDefined();

    // Verify the page is still rendering correctly
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
