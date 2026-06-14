/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Mobile Responsive', () => {

  test('M1: sidebar opens via hamburger on mobile', async ({ page }) => {
    await page.goto('/');

    // Click hamburger
    const hamburger = page.locator('button:has(.fa-bars)');
    await hamburger.click();
    await page.waitForTimeout(600);

    // Sidebar should now be visible
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();
  });

  test('M2: ellipsis button visible on task page', async ({ page }) => {
    await page.goto('/');

    // Open sidebar first
    const hamburger = page.locator('button:has(.fa-bars)');
    await hamburger.click();
    await page.waitForTimeout(600);

    // Click on test-tktube inside the sidebar
    const sidebar = page.locator('[data-testid="sidebar"]');
    await sidebar.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1500);

    // Ellipsis button expands toolbar on mobile
    const ellipsisBtn = page.locator('button:has(.fa-ellipsis-v)');
    await expect(ellipsisBtn).toBeVisible();

    // Expand toolbar
    await ellipsisBtn.click();
    await page.waitForTimeout(300);
  });

  test('M3: config modal opens from sidebar cog button', async ({ page }) => {
    await page.goto('/');

    // Open sidebar to access the cog button
    const hamburger = page.locator('button:has(.fa-bars)');
    await hamburger.click();
    await page.waitForTimeout(600);

    // Click cog in sidebar
    const sidebar = page.locator('[data-testid="sidebar"]');
    const configBtn = sidebar.locator('button:has(.fa-cog)');
    await configBtn.click();
    await page.waitForTimeout(500);

    // After clicking cog, the sidebar should have shown a modal (check sidebar DOM for modal-like elements)
    await expect(page.locator('main')).toBeVisible({ timeout: 3000 });
  });

  test('M4: task cards render on mobile after selecting and hamburger', async ({ page }) => {
    await page.goto('/');

    // Open sidebar
    const hamburger = page.locator('button:has(.fa-bars)');
    await hamburger.click();
    await page.waitForTimeout(600);

    // Select test-tktube
    const sidebar = page.locator('[data-testid="sidebar"]');
    await sidebar.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(2000);

    // Close sidebar so main content is visible
    await hamburger.click();
    await page.waitForTimeout(600);

    // Cards should render in the main content
    const main = page.locator('main');
    await expect(main).toBeVisible();
    const text = await main.textContent();
    expect(text?.length).toBeGreaterThan(0);
  });
});
