/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Lighthouse Audits', () => {

  test('L1: basic accessibility checks via page context', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Check that the page has proper ARIA landmarks
    const main = page.locator('main');
    const sidebar = page.locator('[data-testid="sidebar"]');
    const header = page.locator('h1');

    await expect(header).toBeVisible();
    await expect(main).toBeVisible();
    await expect(sidebar).toBeVisible();
  });

  test('L2: check image alt attributes', async ({ page }) => {
    await page.goto('/');

    // Select test-tktube to render object cards with images
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(2000);

    // Verify all images have alt attributes (pass if no images exist)
    const images = page.locator('img');
    const count = await images.count();
    let withAlt = 0;
    let withoutAlt = 0;
    for (let i = 0; i < count; i++) {
      const alt = await images.nth(i).getAttribute('alt');
      if (alt !== null && alt !== '') {
        withAlt++;
      } else {
        withoutAlt++;
      }
    }
    console.log(`Images: ${count}, with alt: ${withAlt}, without alt: ${withoutAlt}`);
    // Either there are no images, or all have alt text
    expect(withoutAlt).toBeLessThanOrEqual(0);
  });

  test('L3: check viewport meta tag', async ({ page }) => {
    await page.goto('/');

    // Verify viewport meta tag exists and has width=device-width
    const viewportMeta = page.locator('meta[name="viewport"]');
    const count = await viewportMeta.count();
    expect(count).toBe(1);

    if (count === 1) {
      const content = await viewportMeta.getAttribute('content');
      expect(content).toContain('width=device-width');
    }
  });
});
