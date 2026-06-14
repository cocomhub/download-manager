/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { defineConfig, devices } from '@playwright/test';

const TEST_PORT = parseInt(process.env.TEST_PORT || '19199', 10);

export default defineConfig({
  testDir: './specs',
  timeout: 45000,
  expect: {
    timeout: 15000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: [
    ['html', { outputFolder: 'playwright-report' }],
    ['list'],
  ],
  use: {
    baseURL: `http://localhost:${TEST_PORT}`,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'desktop',
      testDir: './specs',
      testIgnore: ['**/mobile.spec.ts'],
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: 'mobile',
      testDir: './specs',
      testMatch: ['**/mobile.spec.ts'],
      use: {
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
        userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1',
        hasTouch: true,
      },
    },
    {
      name: 'firefox',
      testDir: './specs',
      testMatch: ['**/task-lifecycle.spec.ts', '**/error-boundary.spec.ts', '**/realtime-updates.spec.ts', '**/cross-browser-visual.spec.ts', '**/accessibility.spec.ts', '**/network-resilience.spec.ts'],
      use: {
        browserName: 'firefox',
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: 'webkit',
      testDir: './specs',
      testMatch: ['**/task-lifecycle.spec.ts', '**/error-boundary.spec.ts', '**/realtime-updates.spec.ts'],
      use: {
        browserName: 'webkit',
        viewport: { width: 1440, height: 900 },
      },
    },
  ],
  globalSetup: './helpers/global-setup.ts',
  globalTeardown: './helpers/global-teardown.ts',
});
