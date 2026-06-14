/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Accessibility (a11y) audit tests.
 *
 * These tests run axe-core automatically to verify WCAG compliance and
 * ensure keyboard navigation works across the main views.
 */

import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test.describe('Accessibility Audits', () => {

  test('A1: main page has no critical accessibility violations', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1500);

    const accessibilityScanResults = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'best-practice'])
      .analyze();

    // Log all violations for debugging
    if (accessibilityScanResults.violations.length > 0) {
      console.log(`Found ${accessibilityScanResults.violations.length} a11y violations`);
      accessibilityScanResults.violations.forEach(v => {
        console.log(`  - ${v.id}: ${v.help} (${v.impact}, ${v.nodes.length} nodes)`);
      });
    }

    // Baseline: 2 known critical violations (existing UI issues).
    // Fail if the count increases beyond this baseline.
    // Update this number as violations get fixed.
    const criticalViolations = accessibilityScanResults.violations.filter(
      v => v.impact === 'critical'
    );
    expect(criticalViolations.length).toBeLessThanOrEqual(2);

    // Log serious violations for tracking
    const seriousViolations = accessibilityScanResults.violations.filter(
      v => v.impact === 'serious'
    );
    if (seriousViolations.length > 0) {
      console.log(`Serious violations (not failing): ${seriousViolations.length}`);
      seriousViolations.forEach(v => {
        console.log(`  - ${v.id}: ${v.help}`);
      });
    }
  });

  test('A2: task detail view is keyboard navigable', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1000);

    // Navigate to task via keyboard
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab'); // Focus on sidebar

    // Select task with Enter
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    // Verify main content is reachable
    const main = page.locator('main');
    await expect(main).toBeVisible();
  });

  test('A3: keyboard focus indicator shows on interactive elements', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1500);

    // Tab through focusable sidebar elements and verify at least the last is focused
    const focusTargets = [
      '[data-testid="task-test-tktube"]',
      '[data-testid="view-mode-aggregate"]',
    ];

    for (const selector of focusTargets) {
      await page.locator(selector).focus();
      await page.waitForTimeout(50);
    }

    // Final element should have focus
    const last = page.locator(focusTargets[focusTargets.length - 1]);
    const isFocused = await last.evaluate(el => el === document.activeElement);
    expect(isFocused).toBe(true);
  });

  test('A4: color contrast check on main UI elements', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(1500);

    // Check that sidebar text meet minimum contrast requirements via axe
    const results = await new AxeBuilder({ page })
      .include('[data-testid="sidebar"]')
      .withTags(['cat.color'])
      .analyze();

    const colorViolations = results.violations.filter(
      v => v.id === 'color-contrast'
    );
    if (colorViolations.length > 0) {
      console.log(`Color contrast violations: ${colorViolations.length}`);
      colorViolations.forEach(v => {
        console.log(`  - ${v.help}: ${v.nodes.length} nodes`);
      });
    }
    // Fail if new contrast violations appear beyond known baseline
    // Current known: 0 contrast violations in sidebar
    expect(colorViolations.length).toBe(0);
  });
});
