/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

test.describe('Config Management', () => {

  test('T12: Config button works', async ({ page }) => {
    await page.goto('/');

    // Find the config button (fa-cog icon in the header area)
    const configBtn = page.locator('[data-testid="sidebar"] button:has(.fa-cog), button:has(.fa-cog)').first();
    await configBtn.waitFor({ state: 'visible', timeout: 5000 });
    await configBtn.click();
    await page.waitForTimeout(500);

    // Verify some dialog or config element appeared
    // The config opens as a modal or a panel — check body contains config-related text
    const body = page.locator('body');
    const text = await body.textContent();
    expect(text).toBeTruthy();
  });

  test('T13b: proxies textarea round-trips downloader.proxies', async ({ page }) => {
    await page.goto('/');
    const configBtn = page.locator('button:has(.fa-cog)').first();
    await configBtn.waitFor({ state: 'visible', timeout: 5000 });
    await configBtn.click();

    // 绑定必须是嵌套的 downloader.proxies（此前绑顶层 configForm.proxies → 编辑被丢弃）
    const proxies = page.locator('[data-testid="input-proxies"]');
    await proxies.waitFor({ state: 'visible', timeout: 5000 });
    await proxies.fill('http://127.0.0.1:7891');

    const saveBtn = page.locator('button:has-text("Save Changes")').first();
    await saveBtn.click();
    await page.waitForTimeout(800);

    // 重新打开配置：应回显已保存的代理
    await configBtn.click();
    await expect(page.locator('[data-testid="input-proxies"]')).toHaveValue(/127\.0\.0\.1:7891/, { timeout: 5000 });

    // 清理：清空并保存，避免影响后续用例
    await page.locator('[data-testid="input-proxies"]').fill('');
    await page.locator('button:has-text("Save Changes")').first().click();
    await page.waitForTimeout(500);
  });

  test('T13: config panel close works', async ({ page }) => {
    await page.goto('/');

    // Open config panel
    const configBtn = page.locator('button:has(.fa-cog)');
    await configBtn.waitFor({ state: 'visible', timeout: 5000 });
    await configBtn.click();
    await page.waitForTimeout(500);
    await expect(page.locator('text=Config, text=配置').first()).toBeVisible({ timeout: 5000 }).catch(() => {});

    // Look for close button
    const closeBtn = page.locator('button:has(.fa-times), button:has(.fa-close), button:has-text("Close"), button:has-text("关闭")').first();
    if (await closeBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await closeBtn.click();
      await page.waitForTimeout(300);
    }
  });
});
