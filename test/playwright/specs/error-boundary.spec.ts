/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { apiPost, apiPostUI, apiGetUI, UI_ONLY_PORT } from '../helpers/api';

test.describe('UI-only Mode & Error Boundaries', () => {

  test('T14a: full mode has write buttons enabled', async ({ page }) => {
    await page.goto('/');

    // Verify navigation elements in full mode
    await expect(page.locator('[data-testid="view-mode-downloads"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-aggregate"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-mode-dashboard"]')).toBeVisible();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Select a task first to reveal the write buttons
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Write buttons should be enabled in full mode
    await expect(page.locator('[data-testid="btn-retry-all"]')).not.toBeDisabled();
  });

  test('T14b: UI-only page loads correctly', async ({ page }) => {
    // Open UI-only server page
    await page.goto(`http://localhost:${UI_ONLY_PORT}/`);

    // Verify navigation elements exist
    await expect(page.locator('[data-testid="view-mode-downloads"]')).toBeVisible();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Check runtime endpoint to confirm it's UI-only mode
    const runtime = await apiGetUI('/api/runtime');
    expect(runtime).toBeDefined();
  });

  test('T14c: UI-only API returns 405 on write endpoints', async () => {
    // UI-only server: write should reject with 405
    await expect(apiPostUI('/api/tasks/test-tktube/cancel'))
      .rejects.toThrow(/405/);
  });
});
