/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { startServer, startUIOnlyServer } from './server';

async function globalSetup() {
  console.log('[globalSetup] Starting test server...');
  await startServer('full');
  console.log('[globalSetup] Server ready.');

  console.log('[globalSetup] Starting UI-only server...');
  await startUIOnlyServer();
  console.log('[globalSetup] UI-only server ready.');
}

export default globalSetup;
