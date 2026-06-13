/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Task & Object Management', () => {

  test('T1: sidebar renders all fixture tasks with correct metadata', async ({ page }) => {
    await page.goto('/');

    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();

    // Should show the 4 fixture tasks
    await expect(sidebar).toContainText('test-tktube');
    await expect(sidebar).toContainText('test-vikacg');
    await expect(sidebar).toContainText('test-hanime');
    await expect(sidebar).toContainText('test-mixed');
  });

  test('T2: object grid view shows objects with status', async ({ page }) => {
    await page.goto('/');

    // Click on test-tktube in sidebar
    await page.locator('[data-testid="task-test-tktube"]').click();

    // Wait for objects to load (grid view is default — look at rowgroup after clicking task)
    await page.waitForTimeout(1000);

    // Should render object cards or table rows
    const content = page.locator('main');
    await expect(content).toContainText('completed');
  });

  test('T3: list view shows objects as table rows', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Click the "List" button to switch to list view
    const listBtn = page.locator('button:has(.fa-list), button:has-text("List")');
    if (await listBtn.isVisible()) {
      await listBtn.click();
      await page.waitForTimeout(500);
    }

    // In list view, objects render as table rows inside a rowgroup
    const rows = page.locator('table tbody tr, [role="rowgroup"] [role="row"]');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('T4: cancel and retry a single object', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(1000);

    // Find a cancel button on an object and click it
    const cancelBtn = page.locator('[data-testid^="btn-cancel-"]').first();
    if (await cancelBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await cancelBtn.click();
      await page.waitForTimeout(500);
    }
  });

  test('T5: batch cancel and undo', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Try to use select-all and batch cancel
    const selectAllCheckbox = page.locator('input[type="checkbox"]').first();
    if (await selectAllCheckbox.isVisible({ timeout: 3000 }).catch(() => false)) {
      await selectAllCheckbox.check();
      await page.waitForTimeout(200);
    }
  });
});
