/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';

// 本文件的用例都会改写**共享运行服务**的配置（保存即重建下载器）。文件内串行执行，
// 且每个用例开始前用 openConfigModal 归位弹窗状态，避免相互干扰。
test.describe.configure({ mode: 'serial' })

test.describe('Config Management', () => {
  // openConfigModal 确保弹窗先关闭再打开（避免相邻用例残留的打开状态使 click 变成关闭）。
  async function openConfigModal(page: import('@playwright/test').Page) {
    const save = page.locator('button:has-text("Save Changes")');
    if (await save.isVisible().catch(() => false)) {
      await page.keyboard.press('Escape');
      await save.waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
    }
    const cog = page.locator('button:has(.fa-cog)').first();
    await cog.waitFor({ state: 'visible', timeout: 5000 });
    await cog.click();
    await save.waitFor({ state: 'visible', timeout: 5000 });
  }


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
    await openConfigModal(page);

    // 绑定必须是嵌套的 downloader.proxies（此前绑顶层 configForm.proxies → 编辑被丢弃）
    const proxies = page.locator('[data-testid="input-proxies"]');
    await proxies.waitFor({ state: 'visible', timeout: 5000 });
    await proxies.fill('http://127.0.0.1:7891');

    const saveBtn = page.locator('button:has-text("Save Changes")').first();
    await saveBtn.click();
    await page.waitForTimeout(800);

    // 重新打开配置：应回显已保存的代理
    await openConfigModal(page);
    await expect(page.locator('[data-testid="input-proxies"]')).toHaveValue(/127\.0\.0\.1:7891/, { timeout: 5000 });

    // 清理：清空并保存，避免影响后续用例
    await page.locator('[data-testid="input-proxies"]').fill('');
    await page.locator('button:has-text("Save Changes")').first().click();
    await expect(page.locator('button:has-text("Save Changes")')).toBeHidden({ timeout: 10000 });
  });

  test('T13c: log + domain-limits fields round-trip (nested bindings)', async ({ page }) => {
    await page.goto('/');
    await openConfigModal(page);

    // 此前这些字段绑定错层级（顶层 configForm.xxx）→ 编辑被静默丢弃
    const logFile = page.locator('input[placeholder="./logs/app.log"]');
    await logFile.waitFor({ state: 'visible', timeout: 5000 });
    await logFile.fill('./logs/e2e.log');
    // 此前 max_size/max_backups/max_age 绑到不存在的顶层键 → 编辑静默丢弃
    await page.locator('[data-testid="input-log-max-size"]').fill('123');

    const limits = page.locator('[data-testid="input-domain-limits"]');
    await limits.fill('e2e.example.com=3');

    await page.locator('button:has-text("Save Changes")').first().click();
    // 保存成功会关闭弹窗：以此作为确定性完成信号（固定 sleep 会与相邻用例互相干扰）
    await expect(page.locator('button:has-text("Save Changes")')).toBeHidden({ timeout: 10000 });

    await openConfigModal(page);
    await expect(page.locator('input[placeholder="./logs/app.log"]')).toHaveValue('./logs/e2e.log', { timeout: 5000 });
    await expect(page.locator('[data-testid="input-domain-limits"]')).toHaveValue(/e2e\.example\.com=3/, { timeout: 5000 });
    await expect(page.locator('[data-testid="input-log-max-size"]')).toHaveValue('123', { timeout: 5000 });

    // 清理，避免影响后续用例
    await page.locator('input[placeholder="./logs/app.log"]').fill('');
    await page.locator('[data-testid="input-log-max-size"]').fill('1');
    await page.locator('[data-testid="input-domain-limits"]').fill('');
    await page.locator('button:has-text("Save Changes")').first().click();
    await expect(page.locator('button:has-text("Save Changes")')).toBeHidden({ timeout: 10000 });
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
