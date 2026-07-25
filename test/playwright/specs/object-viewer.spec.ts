/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Object viewer E2E tests — verify that all task object click actions
 * render in-page modals rather than opening new pages.
 *
 * Uses the "full" fixture (4 tasks: test-tktube, test-vikacg, test-hanime, test-mixed).
 */

import { test, expect } from '@playwright/test';

test.describe('Object Viewer', () => {

  test('T1: click tktube completed video object opens video player modal', async ({ page }) => {
    // Register pageerror listener to catch JS errors
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Click the cover area of the first object card to trigger handleCardClick
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });

    // Click the cover image area (not buttons)
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();
    await page.waitForTimeout(500);

    // Should open video player modal (fixed overlay with video)
    const videoModal = page.locator('.fixed.inset-0.bg-black');
    await expect(videoModal).toBeVisible({ timeout: 3000 });

    // Should have a video element inside
    const videoEl = videoModal.locator('video');
    await expect(videoEl).toBeVisible({ timeout: 3000 });

    // Close the modal
    const closeBtn = videoModal.locator('button:has(.fa-times)').first();
    await closeBtn.click();
    await page.waitForTimeout(500);
    await expect(videoModal).not.toBeVisible({ timeout: 3000 });

    // Verify no JS errors
    expect(errors.length).toBe(0);
  });

  test('T2: click vikacg completed object opens image gallery', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-vikacg"]').click();
    await expect(page.locator('h2:has-text("test-vikacg")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Click the first object card
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();
    await page.waitForTimeout(500);

    // Should open the image gallery modal (overlay with bg-opacity-70)
    const modal = page.locator('.fixed.inset-0.bg-black.bg-opacity-70');
    await expect(modal).toBeVisible({ timeout: 3000 });

    // Should have an image inside
    const img = modal.locator('img').first();
    await expect(img).toBeVisible({ timeout: 3000 });

    // Close via close button
    const closeBtn = modal.locator('button:has(.fa-times)').first();
    await closeBtn.click();
    await page.waitForTimeout(500);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  test('T3: click hanime completed object opens Hanime viewer', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-hanime"]').click();
    await expect(page.locator('h2:has-text("test-hanime")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Click the first object card
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();
    await page.waitForTimeout(500);

    // Should open the Hanime viewer modal
    const modal = page.locator('.fixed.inset-0.bg-black.bg-opacity-70');
    await expect(modal).toBeVisible({ timeout: 3000 });

    // Should have header with "Hanime" or title
    const header = modal.locator('h3');
    await expect(header).toBeVisible({ timeout: 3000 });

    // Close
    const closeBtn = modal.locator('button:has(.fa-times)').first();
    await closeBtn.click();
    await page.waitForTimeout(500);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  test('T4: click "详情" button on completed object shows info modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    // Use test-tktube — has completed objects
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Find the "详情" button on a completed non-video object
    const detailBtn = page.locator('button:has-text("详情")');
    if (await detailBtn.count() > 0) {
      await detailBtn.first().click();
      await page.waitForTimeout(500);

      // Should open the default info modal (BaseViewer)
      const modal = page.locator('.fixed.inset-0.bg-black.bg-opacity-70');
      await expect(modal).toBeVisible({ timeout: 3000 });

      // Close
      const closeBtn = modal.locator('button:has-text("关闭")').first();
      await closeBtn.click();
      await page.waitForTimeout(500);
      await expect(modal).not.toBeVisible({ timeout: 3000 });
    }

    expect(errors.length).toBe(0);
  });

  test('T5: modal close button returns page to normal state', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Click first object card
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();
    await page.waitForTimeout(500);

    // Modal should be visible
    const modal = page.locator('.fixed.inset-0.bg-black');
    await expect(modal).toBeVisible({ timeout: 3000 });

    // Close the modal
    // Video player modal has different close button location
    const closeBtn = modal.locator('button:has(.fa-times)').first();
    await closeBtn.click();
    await page.waitForTimeout(500);

    // After close, the main content area should be visible without overlay
    const main = page.locator('main');
    await expect(main).toBeVisible({ timeout: 3000 });

    // Verify no overlay remains
    const overlays = await page.locator('.fixed.inset-0.bg-black').count();
    expect(overlays).toBe(0);

    expect(errors.length).toBe(0);
  });

  test('T6: no page navigation occurs when clicking objects', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    // Track the current URL to detect navigation
    let currentUrl = '';
    page.on('load', () => { currentUrl = page.url(); });

    await page.goto('/');
    const baseUrl = page.url();
    currentUrl = baseUrl;

    // Click tktube object — should not navigate away
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();
    await page.waitForTimeout(500);

    // Close the modal if it opened
    const modal = page.locator('.fixed.inset-0.bg-black').first();
    if (await modal.isVisible()) {
      const closeBtn = modal.locator('button:has(.fa-times)').first();
      await closeBtn.click();
      await page.waitForTimeout(500);
    }

    // Verify we never navigated away from the app
    expect(currentUrl).toBe(baseUrl);

    expect(errors.length).toBe(0);
  });
});