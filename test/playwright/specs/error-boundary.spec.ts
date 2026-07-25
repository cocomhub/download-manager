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

  test('T14d: no uncaught JS errors on task selection across all types', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    // Wait for sidebar to render all tasks
    await expect(page.locator('[data-testid="task-test-tktube"]')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(500);

    // Select a tktube task — triggers loadTaskUI for a registered type
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Select a vikacg task — different task type
    await page.locator('[data-testid="task-test-vikacg"]').click();
    await page.waitForTimeout(1000);

    // Select a hanime task — different task type
    await page.locator('[data-testid="task-test-hanime"]').click();
    await page.waitForTimeout(1000);

    // Select a mock-type task (test-mixed) — triggers 404 for unregistered UI type
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(1000);

    // Assert no JS errors occurred
    expect(errors).toEqual([]);
  });

  test('T14e: no uncaught JS errors on new task form type switch', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Open the new task dialog
    const addBtn = page.getByText('新建任务');
    await addBtn.click();
    await page.waitForTimeout(500);

    // Switch between task types in the form — triggers renderForm switching
    const typeSelect = page.locator('select').filter({ has: page.locator('option[value="url_list"]') });
    if (await typeSelect.isVisible()) {
      await typeSelect.selectOption('tktube');
      await page.waitForTimeout(500);
      await typeSelect.selectOption('url_list');
      await page.waitForTimeout(500);
    }

    expect(errors).toEqual([]);
  });
});
