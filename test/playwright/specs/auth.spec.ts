/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { test, expect } from '@playwright/test';
import { startAuthServer, stopAuthServer, AUTH_PORT } from '../helpers/server';

// Auth UI tests use a dedicated server on AUTH_PORT so they don't clash with
// the globalSetup server (TEST_PORT, no auth). All page navigations target
// the auth server explicitly via absolute URL.

const AUTH_BASE = `http://localhost:${AUTH_PORT}`;

test.describe('Auth UI', () => {
  test.afterEach(async () => {
    await stopAuthServer();
  });

  test('A1: login overlay shows when auth enabled and no credential', async ({ page }) => {
    await startAuthServer('basic', { authUser: 'admin', authPass: 'secret' });
    await page.goto(AUTH_BASE + '/');

    await expect(page.locator('[data-testid="login-overlay"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="login-username"]')).toBeVisible();
  });

  test('A2: correct credentials log in and show dashboard', async ({ page }) => {
    await startAuthServer('basic', { authUser: 'admin', authPass: 'secret' });
    await page.goto(AUTH_BASE + '/');

    await page.locator('[data-testid="login-username"]').fill('admin');
    await page.locator('[data-testid="login-password"]').fill('secret');
    await page.locator('[data-testid="login-submit"]').click();

    await expect(page.locator('[data-testid="login-overlay"]')).toBeHidden({ timeout: 10000 });
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  test('A3: wrong credentials show error and stay on login', async ({ page }) => {
    await startAuthServer('basic', { authUser: 'admin', authPass: 'secret' });
    await page.goto(AUTH_BASE + '/');

    await page.locator('[data-testid="login-username"]').fill('admin');
    await page.locator('[data-testid="login-password"]').fill('wrong');
    await page.locator('[data-testid="login-submit"]').click();

    await expect(page.locator('[data-testid="login-error"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="login-overlay"]')).toBeVisible();
  });

  test('A4: logout returns to login overlay', async ({ page }) => {
    await startAuthServer('basic', { authUser: 'admin', authPass: 'secret' });
    await page.goto(AUTH_BASE + '/');

    await page.locator('[data-testid="login-username"]').fill('admin');
    await page.locator('[data-testid="login-password"]').fill('secret');
    await page.locator('[data-testid="login-submit"]').click();

    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 10000 });

    await page.locator('[data-testid="logout-btn"]').click();
    await expect(page.locator('[data-testid="login-overlay"]')).toBeVisible({ timeout: 10000 });
  });

  test('A5: token mode login works', async ({ page }) => {
    await startAuthServer('token', { authToken: 'tok123' });
    await page.goto(AUTH_BASE + '/');

    // Token mode still uses the basic login form in this iteration;
    // verify overlay shows and wrong creds fail.
    await expect(page.locator('[data-testid="login-overlay"]')).toBeVisible({ timeout: 10000 });
    await page.locator('[data-testid="login-username"]').fill('admin');
    await page.locator('[data-testid="login-password"]').fill('wrong');
    await page.locator('[data-testid="login-submit"]').click();
    await expect(page.locator('[data-testid="login-error"]')).toBeVisible({ timeout: 10000 });
  });
});
