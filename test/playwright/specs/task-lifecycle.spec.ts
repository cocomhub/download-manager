/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Task & Object Management', () => {

  // T1-T3 are read-only tests, can run in parallel
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

    // Wait for objects to load by waiting for the content area
    await expect(page.locator('main')).toContainText('completed', { timeout: 10000 });

    // Click the "List" button to switch to list view
    const listBtn = page.locator('button:has(.fa-list), button:has-text("List")');
    await listBtn.waitFor({ state: 'visible', timeout: 5000 });
    await listBtn.click();
    await page.waitForTimeout(500);

    // In list view, objects render as table rows inside a rowgroup
    const rows = page.locator('table tbody tr, [role="rowgroup"] [role="row"]');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('T4: cancel a single object', async ({ page }) => {
    await page.goto('/');
    // Use test-mixed task — it has pending objects with cancel buttons visible
    await page.locator('[data-testid="task-test-mixed"]').click();

    // Wait for objects to load
    await expect(page.locator('main')).toContainText('test-mixed', { timeout: 10000 });

    // Find a cancel button on a pending/downloading object and click it
    const cancelBtn = page.locator('[data-testid^="btn-cancel-"]').first();
    await cancelBtn.waitFor({ state: 'visible', timeout: 5000 });
    await cancelBtn.click();
    await page.waitForTimeout(500);
  });

  test('T5: batch select', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('main')).toContainText('completed', { timeout: 10000 });

    // Try to use select-all checkbox
    const selectAllCheckbox = page.locator('input[type="checkbox"]').first();
    await selectAllCheckbox.waitFor({ state: 'visible', timeout: 5000 });
    await selectAllCheckbox.check();
    await page.waitForTimeout(200);

    // Verify the batch cancel button in sidebar became enabled
    // (sidebar has "取消选中" — there are 2 matching elements, use the sidebar one)
    const sidebarCancelBtn = page.locator('[data-testid="sidebar"] button:has-text("取消选中")');
    await expect(sidebarCancelBtn).toBeEnabled({ timeout: 3000 });
  });
});
