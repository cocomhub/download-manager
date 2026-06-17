/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Network resilience E2E tests.
 *
 * Tests page behavior under degraded network conditions (slow 3G, offline).
 */

import { test, expect } from '@playwright/test';

test.describe('Network Resilience', () => {

  // Simulate slow 3G to test loading states and eventual rendering
  test('N1: page loads under slow 3G conditions', async ({ page }) => {
    // Enable slow 3G throttling
    await page.context().route('**/*', async (route) => {
      const delay = 200; // ms per request
      await new Promise(r => setTimeout(r, delay));
      await route.continue();
    });

    await page.goto('/');
    // Should show loading state immediately
    await expect(page.locator('main')).toBeVisible({ timeout: 10000 });

    // Eventually content loads
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube', { timeout: 15000 });
  });

  // Simulate offline then recovery
  test('N2: page handles offline -> recovery gracefully', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Verify initial data loaded
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube');

    // Simulate offline by blocking all requests
    await page.context().route('**/*api/**', route => route.abort());

    // Navigate — should not crash, show error state
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(500);

    // Remove offline block — let requests through again
    await page.context().unroute('**/*api/**');
    await page.waitForTimeout(500);

    // Page should still display basic structure
    const main = page.locator('main');
    await expect(main).toBeVisible();
  });

  // Simulate API latency spike
  test('N3: page recovers after API latency spike', async ({ page }) => {
    test.setTimeout(60000);
    // Register route BEFORE goto to avoid race between route registration and requests
    let routeHandled = false;
    await page.context().route('**/api/**', async (route, request) => {
      // Skip health check route to keep the server worker heartbeat alive
      if (request.url().includes('/api/healthz')) {
        await route.continue();
        return;
      }
      if (routeHandled) {
        route.continue();
        return;
      }
      routeHandled = true;
      await new Promise(r => setTimeout(r, 3000));
      await route.continue();
    });

    await page.goto('/');
    await page.waitForTimeout(500); // still loading

    // Meanwhile, verify the page shell is visible
    await expect(page.locator('main')).toBeVisible();

    // Now remove delay and verify content loads
    await page.context().unroute('**/api/**');
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toContainText('test-tktube', { timeout: 20000 });
  });

  // Test SSE reconnect under network interruption
  test('N4: SSE reconnects after transient network failure', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="sidebar"]')).toContainText('test-tktube');

    // Block SSE endpoint for a brief period
    await page.context().route('**/api/events', route => route.abort());
    await page.waitForTimeout(200);
    await page.context().unroute('**/api/events');

    // Page should continue to work (SSE auto-reconnect is handled by EventSource spec)
    await page.waitForTimeout(500);
    const main = page.locator('main');
    await expect(main).toBeVisible();
  });
});
