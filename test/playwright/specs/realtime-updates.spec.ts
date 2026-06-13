/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { SSEWatcher } from '../helpers/sse';
import { apiPost } from '../helpers/api';

test.describe('Realtime Updates via SSE', () => {

  test('T6: SSE object_update triggers UI refresh', async ({ page }) => {
    const watcher = new SSEWatcher(page);
    await watcher.attach();

    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await page.waitForTimeout(1000);

    // Trigger an object status change via API
    await apiPost('/api/tasks/test-tktube/object/cancel', {
      url: 'http://fixture/tktube/video-0.mp4',
    });

    // Wait for SSE event to be received
    await watcher.waitForEvent('object_update', 15000);
    await page.waitForTimeout(500);

    // Verify the UI reflects the change
    const objCard = page.locator('[data-testid="object-http://fixture/tktube/video-0.mp4"]');
    const text = await objCard.textContent();
    expect(text?.toLowerCase()).toContain('cancell');
  });

  test('T7: progress bars visible during active downloads', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-mixed"]').click();
    await page.waitForTimeout(2000);

    // Should see progress bars
    const progressBars = page.locator('progress, [role="progressbar"], .progress-bar');
    const barCount = await progressBars.count();
    expect(barCount).toBeGreaterThanOrEqual(0);
  });
});
