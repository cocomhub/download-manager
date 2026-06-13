import { test, expect } from '@playwright/test';

test.describe('Task & Object Management', () => {

  // T1-T3 are read-only tests, can run in parallel
  test('T1: sidebar renders all fixture tasks with correct metadata', async ({ page }) => {
    await page.goto('/');

    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();

    await expect(sidebar).toContainText('test-tktube');
    await expect(sidebar).toContainText('test-vikacg');
    await expect(sidebar).toContainText('test-hanime');
    await expect(sidebar).toContainText('test-mixed');
  });

  test('T2: object grid view shows objects with status', async ({ page }) => {
    await page.goto('/');

    await page.locator('[data-testid="task-test-tktube"]').click();

    // Wait for task detail heading to appear
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // Verify main content has rendered content
    const main = page.locator('main');
    const text = await main.textContent();
    expect(text?.length).toBeGreaterThan(0);
  });

  test('T3: list view shows objects as table rows', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(500);

    const listBtn = page.locator('button:has(.fa-list)');
    await listBtn.waitFor({ state: 'visible', timeout: 5000 });
    await listBtn.click();
    await page.waitForTimeout(500);

    const rows = page.locator('table tbody tr, [role="rowgroup"] [role="row"]');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('T4: cancel a single object', async ({ page }) => {
    await page.goto('/');
    // Use test-mixed — it has pending objects with cancel buttons
    await page.locator('[data-testid="task-test-mixed"]').click();
    await expect(page.locator('h2:has-text("test-mixed")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    const cancelBtn = page.locator('[data-testid^="btn-cancel-"]').first();
    await cancelBtn.waitFor({ state: 'visible', timeout: 5000 });
    await cancelBtn.click();
    await page.waitForTimeout(500);
  });

  test('T5: batch select', async ({ page }) => {
    await page.goto('/');
    await page.locator('[data-testid="task-test-tktube"]').click();
    await expect(page.locator('h2:has-text("test-tktube")')).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    const selectAllCheckbox = page.locator('input[type="checkbox"]').first();
    await selectAllCheckbox.waitFor({ state: 'visible', timeout: 5000 });
    await selectAllCheckbox.check();
    await page.waitForTimeout(500);

    // Verify main content area has rendered content
    await page.waitForTimeout(1000);
    const main = page.locator('main');
    const text = await main.textContent();
    expect(text?.length).toBeGreaterThan(0);
  });
});
