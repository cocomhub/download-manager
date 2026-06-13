import { stopServer } from './server';

async function globalTeardown() {
  console.log('[globalTeardown] Stopping test server...');
  await stopServer();
  console.log('[globalTeardown] Server stopped.');
}

export default globalTeardown;
