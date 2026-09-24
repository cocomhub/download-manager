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
let authServerProcess: ChildProcess | null = null;

const UI_ONLY_PORT = TEST_PORT + 1;
const AUTH_PORT = TEST_PORT + 2;

export interface StartServerOptions {
  fixture?: string;
  auth?: 'basic' | 'token';
  authUser?: string;
  authPass?: string;
  authToken?: string;
}

export async function startServer(fixture: string, opts: StartServerOptions = {}): Promise<void> {
  const serverPath = SERVER_BINARY;

  const args = [
    '--port', String(TEST_PORT),
    '--fixture', fixture,
  ];
  if (opts.auth) {
    args.push('--auth', opts.auth);
  }
  if (opts.authUser) {
    args.push('--auth-user', opts.authUser);
  }
  if (opts.authPass) {
    args.push('--auth-pass', opts.authPass);
  }
  if (opts.authToken) {
    args.push('--auth-token', opts.authToken);
  }

  serverProcess = spawn(serverPath, args, {
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

export async function startAuthServer(auth: 'basic' | 'token', opts: { authUser?: string; authPass?: string; authToken?: string } = {}): Promise<void> {
  const serverPath = SERVER_BINARY;
  const args = [
    '--port', String(AUTH_PORT),
    '--fixture', 'full',
    '--auth', auth,
  ];
  if (opts.authUser) args.push('--auth-user', opts.authUser);
  if (opts.authPass) args.push('--auth-pass', opts.authPass);
  if (opts.authToken) args.push('--auth-token', opts.authToken);

  authServerProcess = spawn(serverPath, args, {
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  authServerProcess.stdout?.on('data', (data: Buffer) => {
    console.log(`[auth-server] ${data.toString().trim()}`);
  });
  authServerProcess.stderr?.on('data', (data: Buffer) => {
    console.error(`[auth-server:err] ${data.toString().trim()}`);
  });
  authServerProcess.on('exit', (code) => {
    console.log(`[auth-server] exited with code ${code}`);
    authServerProcess = null;
  });
  await waitForHealthz(AUTH_PORT, 15000);
}

export async function stopAuthServer(): Promise<void> {
  if (authServerProcess) {
    await killProcess(authServerProcess);
    authServerProcess = null;
  }
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
  await stopAuthServer();
}

export { TEST_PORT, UI_ONLY_PORT, AUTH_PORT };

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
