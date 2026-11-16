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
    let batchBody: any = null;
    let triggerBody: any = null;
    let migrationApplied = false;

    await page.route('**/api/tasks/*/remote_save', async (route) => {
      if (route.request().method() === 'POST') {
        triggerBody = JSON.parse(route.request().postData() || '{}');
        await route.fulfill({
          status: 202,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, force: true }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enabled: true, volume: 'baidu', dir: 'hanime/2.5D', objects_total: 1, conflicts: 1 }),
      });
    });
    await page.route('**/api/metrics', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ remote_save: { files_uploaded: 2, files_reused: 1, files_failed: 1, conflicts: 1, bytes_uploaded: 1024, files_skipped: 0, mirror_deleted: 3, cloud_files: 4, trend: [{ minute: 1, files_uploaded: 2, files_failed: 1, conflicts: 1, mirror_deleted: 3 }] } }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/objects', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          objects: [{
            object_url: conflict.object_url, object_id: 1, download_status: 'completed',
            status: 'conflict', files_total: 1, files_done: 0, conflicts: 1, retries: 0,
            last_error: 'checksum_mismatch', play_url: 'https://example.invalid/play.mp4',
          }],
          total: 1,
        }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/migration/apply', async (route) => {
      migrationApplied = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ succeeded: 1, failed: 0, results: [] }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/migration', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [{ object_url: conflict.object_url, path: 'video-0.mp4', old_remote: 'hanime/OLD/video-0.mp4', new_remote: 'hanime/NEW/video-0.mp4' }], total: 1 }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/conflicts', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ conflicts: [conflict], total: 1 }),
      });
    });
    await page.route('**/api/tasks/*/remote_save/conflicts/resolve_batch', async (route) => {
      batchBody = JSON.parse(route.request().postData() || '{}');
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, succeeded: 1, failed: 0, results: [] }),
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
    await expect(page.locator('[data-testid="remote-save-metrics"]')).toContainText('1.00 KiB');
    await expect(page.locator('[data-testid="remote-save-metrics"]')).toContainText('mirror 删除');
    await expect(page.locator('[data-testid="remote-save-trend"]')).toContainText('最近 5 分钟');
    await expect(page.locator('[data-testid="remote-save-migration-item"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="remote-save-object"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="remote-save-objects"]')).toContainText('冲突');
    await expect(page.locator('[data-testid="remote-save-conflict"]')).toHaveCount(1);
    await expect(modal).toContainText('sha256 不一致');
    await expect(modal).toContainText('ABC-001/video-0.mp4');
    await expect(page.locator('[data-testid="remote-save-play"]')).toHaveAttribute('href', 'https://example.invalid/play.mp4');

    await page.locator('[data-testid="remote-save-object-trigger"]').click();
    await expect.poll(() => triggerBody && triggerBody.url).toBe(conflict.object_url);

    await page.locator('[data-testid="remote-save-trigger"]').click();
    await expect.poll(() => triggerBody && triggerBody.force).toBe(true);
    expect(triggerBody.url || '').toBe('');

    page.once('dialog', async (dialog) => { await dialog.accept(); });
    await page.locator('[data-testid="remote-save-migration-apply"]').click();
    await expect.poll(() => migrationApplied).toBe(true);

    await page.locator('[data-testid="remote-save-bulk-skip"]').click();
    await expect.poll(() => batchBody && batchBody.action).toBe('skip');
    expect(batchBody.items).toHaveLength(1);
    expect(batchBody.items[0].path).toBe('video-0.mp4');

    await page.locator('[data-testid="remote-save-reuse"]').click();
    await expect.poll(() => resolveBody && resolveBody.action).toBe('reuse');
    expect(resolveBody.path).toBe('video-0.mp4');
    expect(resolveBody.object_url).toBe(conflict.object_url);
  });

  test('RSP2: empty state when no conflicts', async ({ page }) => {
    await page.route('**/api/tasks/*/remote_save', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ enabled: true, conflicts: 0 }) });
    });
    await page.route('**/api/metrics', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ remote_save: { files_uploaded: 0 } }) });
    });
    await page.route('**/api/tasks/*/remote_save/objects', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ objects: [], total: 0 }) });
    });
    await page.route('**/api/tasks/*/remote_save/migration', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], total: 0 }) });
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
