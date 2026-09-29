/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Batch object operations (P7-2): select objects → retry_batch / delete_batch.
 * Uses the 'full' fixture with task objects; verifies UI wiring to new endpoints.
 */

import { test, expect } from '@playwright/test';
import { TEST_PORT } from '../helpers/api';

test.describe('Batch Object Operations', () => {

  test('BO1: task detail toolbar has batch retry and delete buttons', async ({ page }) => {
    await page.goto('/');
    // 任务列表视图
    await page.locator('[data-testid="view-mode-downloads"]').click();
    // 选中第一个任务（点击任务名文本，触发任务详情工具栏渲染）
    await page.getByText('test-hanime').first().click({ timeout: 15000 }).catch(() => {});
    await page.waitForTimeout(1500);
    // 批量操作条（含批量重试 + 删除选中）应在任务详情工具栏渲染
    await expect(page.locator('button', { hasText: '批量重试' }).first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator('button', { hasText: '删除选中' }).first()).toBeVisible({ timeout: 10000 });
  });

  test('BO2: delete_batch API rejects empty urls', async ({ page }) => {
    // Direct API check: empty urls must be rejected (fail-closed)
    const resp = await globalThis.fetch('http://127.0.0.1:' + TEST_PORT + '/api/tasks/nonexistent/object/delete_batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ urls: [] }),
    });
    expect(resp.status).toBe(400);
  });

  test('BO3: retry_batch API returns per-URL result map', async ({ page }) => {
    const resp = await globalThis.fetch('http://127.0.0.1:' + TEST_PORT + '/api/tasks/nonexistent/object/retry_batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ urls: ['http://x/1'] }),
    });
    const data = await resp.json();
    // Task not found → per-URL error map
    expect(resp.status).toBe(200);
    expect(typeof data['http://x/1']).toBe('string');
  });
});
