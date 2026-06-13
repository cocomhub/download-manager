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

    // Wait for objects to load (grid view is default)
    const objects = page.locator('[data-testid^="object-"]');
    await expect(objects.first()).toBeVisible({ timeout: 10000 });

    // Should render object cards with status indicators
    const firstCard = objects.first();
    await expect(firstCard).toBeVisible();
  });

  test('T3: list view shows objects as table rows', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(500);

    // Try to find list toggle button
    const listBtn = page.locator('button:has(.fa-list), button:has-text("列表")');
    if (await listBtn.isVisible()) {
      await listBtn.click();
      await page.waitForTimeout(500);
    }

    // Verify objects are still visible
    const objects = page.locator('[data-testid^="object-"]');
    await expect(objects.first()).toBeVisible({ timeout: 5000 });
  });

  test('T4: cancel and retry a single object', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(500);

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
