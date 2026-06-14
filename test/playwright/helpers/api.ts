/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import { request, type RequestOptions } from 'http';

const TEST_PORT = parseInt(process.env.TEST_PORT || '19199', 10);
export const UI_ONLY_PORT = TEST_PORT + 1;
export { TEST_PORT };

export async function apiGet<T = any>(path: string): Promise<T> {
  return apiRequest<T>('GET', path, TEST_PORT);
}

export async function apiPost<T = any>(path: string, body?: any): Promise<T> {
  return apiRequest<T>('POST', path, TEST_PORT, body);
}

export async function apiPatch<T = any>(path: string, body?: any): Promise<T> {
  return apiRequest<T>('PATCH', path, TEST_PORT, body);
}

export async function apiGetUI<T = any>(path: string): Promise<T> {
  return apiRequest<T>('GET', path, UI_ONLY_PORT);
}

export async function apiPostUI<T = any>(path: string, body?: any): Promise<T> {
  return apiRequest<T>('POST', path, UI_ONLY_PORT, body);
}

function apiRequest<T>(method: string, path: string, port: number, body?: any): Promise<T> {
  return new Promise((resolve, reject) => {
    const options: RequestOptions = {
      hostname: 'localhost',
      port,
      path,
      method,
      headers: { 'Content-Type': 'application/json' },
    };

    const req = request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        if (res.statusCode && res.statusCode >= 400) {
          reject(new Error(`API ${method} ${path} returned ${res.statusCode}: ${data}`));
          return;
        }
        try {
          resolve(JSON.parse(data));
        } catch {
          resolve(data as unknown as T);
        }
      });
    });

    req.on('error', reject);

    if (body !== undefined) {
      req.write(JSON.stringify(body));
    }
    req.end();
  });
}
