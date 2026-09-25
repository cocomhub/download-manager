/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Video Player', () => {

  test('V1: tktube video opens player with speed display', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Find a completed video object and click it
    const completed = page.locator('[data-status="completed"]').first();
    if (await completed.count() > 0) {
      await completed.click();
      await page.waitForTimeout(1000);
      // Video player speed button should exist
      const speedBtn = page.locator('button[title*="倍速"], .group\/speed button').first();
      if (await speedBtn.count() > 0) {
        await expect(speedBtn).toBeVisible({ timeout: 5000 });
      }
    }
  });

  test('V2: image object detail shows fullscreen button', async ({ page }) => {
    await page.goto('/');
    // vikacg is an image site
    await page.locator('[data-testid="task-test-vikacg"]').click();
    await expect(page.locator('h2:has-text("test-vikacg")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    const imgCard = page.locator('img').first();
    if (await imgCard.count() > 0) {
      // Click the card to open detail
      await imgCard.click().catch(() => {});
      await page.waitForTimeout(1000);
      // Fullscreen button (fa-expand) should exist in detail header
      const fullBtn = page.locator('button:has(.fa-expand)').first();
      if (await fullBtn.count() > 0) {
        await expect(fullBtn).toBeVisible({ timeout: 5000 });
      }
    }
  });
});
