/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { stopServer } from './server';

async function globalTeardown() {
  console.log('[globalTeardown] Stopping test server...');
  await stopServer();
  console.log('[globalTeardown] Server stopped.');
}

export default globalTeardown;
