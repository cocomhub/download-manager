/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Remote-save conflict panel UI wiring. Backend real E2E is covered by the
 * standalone remote-save E2E script; this spec isolates UI state/actions.
 */

import { test, expect } from '@playwright/test';

const conflict = {
  object_url: 'http://fixture/tktube/video-0.mp4',
  object_id: 1,
  path: 'video-0.mp4',
  remote_path: 'hanime/2.5D/ABC-001/video-0.mp4',
  size: 123,
  sha256: 'aaaa',
  remote_sha256: 'bbbb',
  reason: 'checksum_mismatch',
};

test.describe('Remote Save Conflict Panel', () => {
  test('RSP1: renders conflicts and resolves reuse', async ({ page }) => {
    let resolveBody: any = null;

    await page.route('**/api/tasks/*/remote_save', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enabled: true, volume: 'baidu', dir: 'hanime/2.5D', objects_total: 1, conflicts: 1 }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/conflicts', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ conflicts: [conflict], total: 1 }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/conflicts/resolve', async (route) => {
      resolveBody = JSON.parse(route.request().postData() || '{}');
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true }),
      });
    });

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    const btn = page.locator('[data-testid="btn-remote-save"]');
    await expect(btn).toBeVisible({ timeout: 10000 });
    await btn.click();

    const modal = page.locator('[data-testid="remote-save-modal"]');
    await expect(modal).toBeVisible();
    await expect(page.locator('[data-testid="remote-save-conflict"]')).toHaveCount(1);
    await expect(modal).toContainText('sha256 不一致');
    await expect(modal).toContainText('ABC-001/video-0.mp4');

    await page.locator('[data-testid="remote-save-reuse"]').click();
    await expect.poll(() => resolveBody && resolveBody.action).toBe('reuse');
    expect(resolveBody.path).toBe('video-0.mp4');
    expect(resolveBody.object_url).toBe(conflict.object_url);
  });

  test('RSP2: empty state when no conflicts', async ({ page }) => {
    await page.route('**/api/tasks/*/remote_save', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ enabled: true, conflicts: 0 }) });
    });
    await page.route('**/api/tasks/*/remote_save/conflicts', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ conflicts: [], total: 0 }) });
    });
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.locator('[data-testid="btn-remote-save"]').click();
    await expect(page.locator('[data-testid="remote-save-empty"]')).toBeVisible();
  });
});
