/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { spawn, type ChildProcess } from 'child_process';
import { request } from 'http';

const TEST_PORT = parseInt(process.env.TEST_PORT || '19199', 10);
const SERVER_BINARY = process.env.SERVER_BINARY ||
  `../../cmd/playwright-server/playwright-server${process.platform === 'win32' ? '.exe' : ''}`;

let serverProcess: ChildProcess | null = null;
let uiOnlyServerProcess: ChildProcess | null = null;

const UI_ONLY_PORT = TEST_PORT + 1;

export async function startServer(fixture: string): Promise<void> {
  const serverPath = SERVER_BINARY;

  serverProcess = spawn(serverPath, [
    '--port', String(TEST_PORT),
    '--fixture', fixture,
  ], {
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  serverProcess.stdout?.on('data', (data: Buffer) => {
    console.log(`[server] ${data.toString().trim()}`);
  });

  serverProcess.stderr?.on('data', (data: Buffer) => {
    console.error(`[server:err] ${data.toString().trim()}`);
  });

  serverProcess.on('exit', (code) => {
    console.log(`[server] exited with code ${code}`);
    serverProcess = null;
  });

  await waitForHealthz(TEST_PORT, 15000);
}

export async function startUIOnlyServer(): Promise<void> {
  const serverPath = SERVER_BINARY;

  uiOnlyServerProcess = spawn(serverPath, [
    '--port', String(UI_ONLY_PORT),
    '--ui-only',
  ], {
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  uiOnlyServerProcess.stdout?.on('data', (data: Buffer) => {
    console.log(`[ui-only] ${data.toString().trim()}`);
  });

  uiOnlyServerProcess.stderr?.on('data', (data: Buffer) => {
    console.error(`[ui-only:err] ${data.toString().trim()}`);
  });

  uiOnlyServerProcess.on('exit', (code) => {
    console.log(`[ui-only] exited with code ${code}`);
    uiOnlyServerProcess = null;
  });

  await waitForHealthz(UI_ONLY_PORT, 15000);
}

export async function stopServer(): Promise<void> {
  if (serverProcess) {
    await killProcess(serverProcess);
    serverProcess = null;
  }
  if (uiOnlyServerProcess) {
    await killProcess(uiOnlyServerProcess);
    uiOnlyServerProcess = null;
  }
}

export { TEST_PORT, UI_ONLY_PORT };

function killProcess(proc: ChildProcess): Promise<void> {
  return new Promise((resolve) => {
    const killTimer = setTimeout(() => {
      console.log('[server] force kill');
      try { proc.kill('SIGKILL'); } catch { /* ignore */ }
      resolve();
    }, 5000);

    proc.on('exit', () => {
      clearTimeout(killTimer);
      resolve();
    });

    try {
      proc.kill('SIGTERM');
    } catch {
      clearTimeout(killTimer);
      resolve();
    }
  });
}

function waitForHealthz(port: number, timeoutMs: number): Promise<void> {
  const start = Date.now();

  return new Promise((resolve, reject) => {
    const poll = () => {
      if (Date.now() - start > timeoutMs) {
        return reject(new Error(`Server healthz timeout after ${timeoutMs}ms`));
      }

      const req = request({
        hostname: 'localhost',
        port,
        path: '/api/healthz',
        method: 'GET',
        timeout: 1000,
      }, (res) => {
        if (res.statusCode === 200) {
          resolve();
        } else {
          setTimeout(poll, 200);
        }
      });

      req.on('error', () => setTimeout(poll, 200));
      req.end();
    };

    poll();
  });
}
