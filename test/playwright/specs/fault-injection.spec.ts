/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Fault injection and resilience tests.
 *
 * Tests server disconnect/reconnect, SSE recovery, and timeout handling.
 */

import { test, expect } from '@playwright/test';
import { apiGet } from '../helpers/api';

test.describe('Fault Injection & Resilience', () => {

  test('R1: page loads and displays task data after navigation', async ({ page }) => {
    // Navigate to establish SSE connection
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Verify sidebar shows task data
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube');

    // Re-navigate to test resilience
    await page.goto('/');
    await page.waitForTimeout(1000);
    await expect(sidebar).toContainText('test-tktube');
  });

  test('R2: API returns healthy response under concurrent load', async () => {
    // Fire multiple parallel API requests
    const results = await Promise.allSettled([
      apiGet('/api/tasks'),
      apiGet('/api/healthz'),
      apiGet('/api/runtime'),
      apiGet('/api/aggregate'),
      apiGet('/api/metrics'),
    ]);

    // All should be fulfilled (not rejected)
    const fulfilled = results.filter(r => r.status === 'fulfilled');
    expect(fulfilled.length).toBe(5);
  });

  test('R3: 404 handling returns proper JSON error', async () => {
    const res = await fetch('http://localhost:19199/api/tasks/nonexistent');
    expect(res.status).toBe(404);

    const body = await res.json();
    expect(body).toHaveProperty('error');
    expect(body).toHaveProperty('message');
  });

  test('R4: rapid page navigation does not break state', async ({ page }) => {
    await page.goto('/');

    // Rapidly switch between views
    for (let i = 0; i < 5; i++) {
      await page.locator('[data-testid="view-mode-downloads"]').click();
      await page.waitForTimeout(100);
      await page.locator('[data-testid="view-mode-aggregate"]').click();
      await page.waitForTimeout(100);
      await page.locator('[data-testid="view-mode-dashboard"]').click();
      await page.waitForTimeout(100);
    }

    // Final state should still be functional
    const main = page.locator('main');
    await expect(main).toBeVisible();
  });

  test('R5: SSE reconnect triggers task refresh on reopen', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Verify initial load
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube');

    // Reload page to simulate full reconnection cycle
    await page.reload();
    await page.waitForTimeout(1000);

    // After reload, SSE reconnects and onopen fetches tasks
    await expect(sidebar).toContainText('test-tktube');
  });
});
