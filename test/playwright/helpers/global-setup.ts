import { startServer } from './server';

async function globalSetup() {
  console.log('[globalSetup] Starting test server...');
  await startServer('full');
  console.log('[globalSetup] Server ready.');
}

export default globalSetup;
