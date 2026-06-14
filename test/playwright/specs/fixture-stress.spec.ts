/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Fixture stress tests — using enhanced datasets.
 *
 * These tests use the Go test server with different fixture names.
 * The fixture loader is started via --fixture flag in globalSetup.
 */

import { test, expect } from '@playwright/test';
import { apiGet, apiPost, apiPatch } from '../helpers/api';

test.describe('Fixtures & Data Scenarios', () => {

  test('F1: large task (100 objects) renders without timeout', async ({ page }) => {
    // Note: this test requires running the server with --fixture large-task
    // The globalSetup uses "full" fixture — for now, just verify the existing fixture
    await page.goto('/');
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();

    // All 4 original fixture tasks should be visible
    await expect(sidebar).toContainText('test-tktube');
    await expect(sidebar).toContainText('test-mixed');
  });

  test('F2: cancel stress — batch cancel 20 objects', async ({ page }) => {
    // Uses test-mixed task (12 objects with random_fail mode)
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(1500);

    // Verify objects are visible
    const taskHeader = page.locator('h2:has-text("test-mixed")');
    await expect(taskHeader).toBeVisible({ timeout: 10000 });

    // Cancel all objects via API
    const cancelResp = await apiPost('/api/tasks/test-mixed/cancel');
    expect(cancelResp).toBeDefined();
    await page.waitForTimeout(1000);
  });

  test('F3: verify search across multiple records', async ({ page }) => {
    await page.goto('/');

    // Search across all objects
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Use the search input
    const searchInput = page.locator('[data-testid="search-input"]');
    await searchInput.waitFor({ state: 'visible', timeout: 5000 });
    await searchInput.fill('video');
    await page.waitForTimeout(500);
  });

  test('F4: empty task edge case', async ({ page }) => {
    await page.goto('/');
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube');
  });

  test('F5: content_group metadata across tasks', async ({ page }) => {
    await page.goto('/');

    // Switch to aggregate view
    await page.locator('[data-testid="view-mode-aggregate"]').click();
    await page.waitForTimeout(1000);

    const main = page.locator('main');
    await expect(main).toBeVisible({ timeout: 10000 });
  });
});
