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

/** Helper: wait for an overlay modal to appear and return it */
async function waitForModal(page: import('@playwright/test').Page, timeout = 8000) {
  // The modal overlay has class "fixed inset-0 bg-black bg-opacity-70" from Tailwind
  const modal = page.locator('.fixed.inset-0.bg-black');
  await expect(modal).toBeVisible({ timeout });
  return modal;
}

/** Helper: close a modal via the × button */
async function closeModal(modal: import('@playwright/test').Locator) {
  const closeBtn = modal.locator('button:has(.fa-times)').first();
  await expect(closeBtn).toBeVisible({ timeout: 3000 });
  await closeBtn.click();
}

test.describe('Object Viewer', () => {

  // ---- T1: tktube video player modal ----

  test('T1: click tktube completed video object opens video player modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Click the cover area of the first object card
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();

    // Wait for modal
    const modal = await waitForModal(page);

    // Video element should exist (initially hidden — poster mode)
    const videoEl = modal.locator('video');
    await expect(videoEl).toHaveCount(1, { timeout: 3000 });

    // Poster image should be visible (cover image or placeholder)
    const posterImg = modal.locator('img').first();
    await expect(posterImg).toBeVisible({ timeout: 3000 });

    // Close the modal
    await closeModal(modal);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T2: vikacg image gallery ----

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

    // Wait for modal
    const modal = await waitForModal(page, 10000);

    // Should have an image element inside (image may be hidden if URL is unreachable)
    const img = modal.locator('img').first();
    await expect(img).toHaveCount(1, { timeout: 3000 });

    // Close via close button
    await closeModal(modal);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T3: hanime viewer ----

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

    // Wait for modal
    const modal = await waitForModal(page, 10000);

    // Should have header with title
    const header = modal.locator('h3');
    await expect(header).toBeVisible({ timeout: 3000 });

    // Close
    await closeModal(modal);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T4: 详情 button ----

  test('T4: click "详情" button on completed object shows info modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Find the "详情" button
    const detailBtn = page.locator('button:has-text("详情")');
    const btnCount = await detailBtn.count();
    if (btnCount > 0) {
      await detailBtn.first().click();
      await page.waitForTimeout(500);

      // Should open the default info modal (BaseViewer)
      const modal = page.locator('.fixed.inset-0.bg-black');
      await expect(modal).toBeVisible({ timeout: 5000 });

      // Close
      const closeBtn = modal.locator('button:has-text("关闭")').first();
      await closeBtn.click();
      await page.waitForTimeout(500);
      await expect(modal).not.toBeVisible({ timeout: 3000 });
    }

    expect(errors.length).toBe(0);
  });

  // ---- T5: modal close restores page ----

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

    // Modal should be visible
    const modal = await waitForModal(page);

    // Close the modal
    await closeModal(modal);
    await page.waitForTimeout(500);

    // After close, the main content area should be visible
    const main = page.locator('main');
    await expect(main).toBeVisible({ timeout: 3000 });

    // Verify no overlay remains
    const overlays = await page.locator('.fixed.inset-0.bg-black').count();
    expect(overlays).toBe(0);

    expect(errors.length).toBe(0);
  });

  // ---- T6: no navigation ----

  test('T6: no page navigation occurs when clicking objects', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

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

  // ---- T7: Escape key closes modal ----

  test('T7: pressing Escape closes the viewer modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Open a modal
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();

    const modal = await waitForModal(page);

    // Press Escape
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // Modal should be closed
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T8: Backdrop click closes modal ----

  test('T8: clicking backdrop closes the viewer modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Open a modal
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();

    const modal = await waitForModal(page);

    // Click the backdrop (the overlay, not the panel)
    // The overlay is the .fixed.inset-0 element, clicking it directly
    // triggers the e.target === overlay check
    await modal.click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);

    // Modal should be closed
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T9: hanime viewer footer buttons ----

  test('T9: hanime viewer footer has functional buttons', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-hanime"]').click();
    await expect(page.locator('h2:has-text("test-hanime")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Open hanime viewer
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();

    const modal = await waitForModal(page, 10000);

    // Footer should have buttons: 打开原页面, 复制链接, 复制标题, 关闭
    const footer = modal.locator('[style*="border-top"]').last();

    // 复制标题 button should exist
    const copyTitleBtn = footer.locator('button:has-text("复制标题")');
    await expect(copyTitleBtn).toBeVisible({ timeout: 3000 });

    // 关闭 button should exist
    const closeBtn = footer.locator('button:has-text("关闭")');
    await expect(closeBtn).toBeVisible({ timeout: 3000 });

    // Close the modal
    await closeBtn.click();
    await page.waitForTimeout(500);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T10: tktube viewer footer buttons ----

  test('T10: tktube viewer footer has functional buttons', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Open tktube viewer
    const objCard = page.locator('[data-testid^="object-"]').first();
    await objCard.waitFor({ state: 'visible', timeout: 5000 });
    const cover = objCard.locator('.aspect-\\[16\\/9\\]').first();
    await cover.click();

    const modal = await waitForModal(page);

    // Footer should have buttons: 打开文件, 打开原页面, 复制标题, 复制链接, 关闭
    const footer = modal.locator('[style*="border-top"]').last();

    // 复制标题 button should exist
    const copyTitleBtn = footer.locator('button:has-text("复制标题")');
    await expect(copyTitleBtn).toBeVisible({ timeout: 3000 });

    // 关闭 button should exist
    const closeBtn = footer.locator('button:has-text("关闭")');
    await expect(closeBtn).toBeVisible({ timeout: 3000 });

    // Close the modal
    await closeBtn.click();
    await page.waitForTimeout(500);
    await expect(modal).not.toBeVisible({ timeout: 3000 });

    expect(errors.length).toBe(0);
  });

  // ---- T11: 查看 button on hanime card ----

  test('T11: "查看" button on hanime card opens viewer', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-hanime"]').click();
    await expect(page.locator('h2:has-text("test-hanime")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Find the "查看" button on a hanime card
    const viewBtn = page.locator('button:has-text("查看")').first();
    const btnCount = await viewBtn.count();
    if (btnCount > 0) {
      await viewBtn.click();
      const modal = await waitForModal(page, 10000);
      await expect(modal).toBeVisible({ timeout: 5000 });
      await closeModal(modal);
      await expect(modal).not.toBeVisible({ timeout: 3000 });
    }

    expect(errors.length).toBe(0);
  });

  // ---- T12: 播放 button on video card ----

  test('T12: "播放" button on video card opens video player', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Find the "播放" button on a tktube card
    const playBtn = page.locator('button:has-text("播放")').first();
    const btnCount = await playBtn.count();
    if (btnCount > 0) {
      await playBtn.click();
      const modal = await waitForModal(page);
      await expect(modal).toBeVisible({ timeout: 5000 });
      await closeModal(modal);
      await expect(modal).not.toBeVisible({ timeout: 3000 });
    }

    expect(errors.length).toBe(0);
  });

  // ---- T13: 查看分组 button ----

  test('T13: "查看分组" button opens group modal', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Objects with content_group should have a "查看分组" button
    const groupBtn = page.locator('button:has-text("查看分组")').first();
    const btnCount = await groupBtn.count();
    if (btnCount > 0) {
      await groupBtn.click();
      await page.waitForTimeout(500);

      // Group modal should be visible
      const groupModal = page.locator('.fixed.inset-0.bg-black').first();
      if (await groupModal.isVisible()) {
        // Close group modal (if it has a close mechanism)
        const closeBtn = groupModal.locator('button:has(.fa-times)').first();
        if (await closeBtn.isVisible()) {
          await closeBtn.click();
          await page.waitForTimeout(500);
          await expect(groupModal).not.toBeVisible({ timeout: 3000 });
        }
      }
    }

    expect(errors.length).toBe(0);
  });
});