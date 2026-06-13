/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import type { Page } from '@playwright/test';

/**
 * SSEWatcher intercepts EventSource messages on the page for test verification.
 *
 * Usage:
 *   const watcher = new SSEWatcher(page);
 *   await watcher.attach();
 *   // ... trigger an SSE event ...
 *   const data = await watcher.waitForEvent('object_update');
 */
export class SSEWatcher {
  private page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async attach(): Promise<void> {
    await this.page.addInitScript(() => {
      const _log: Array<{ type: string; data: any }> = [];
      const _origAddEventListener = EventSource.prototype.addEventListener;

      EventSource.prototype.addEventListener = function (
        type: string,
        listener: EventListenerOrEventListenerObject | null,
        options?: AddEventListenerOptions | boolean
      ): void {
        if (typeof listener === 'function') {
          const wrapped = (event: Event) => {
            const msgEvent = event as MessageEvent;
            try {
              _log.push({ type, data: JSON.parse(msgEvent.data) });
            } catch {
              _log.push({ type, data: msgEvent.data });
            }
            listener(event);
          };
          _origAddEventListener.call(this, type, wrapped, options);
        } else {
          _origAddEventListener.call(this, type, listener, options);
        }
      };

      // Patch global message handler too
      const _origOnMessage = Object.getOwnPropertyDescriptor(EventSource.prototype, 'onmessage');
      if (_origOnMessage?.configurable) {
        Object.defineProperty(EventSource.prototype, 'onmessage', {
          get() { return this._onmessage; },
          set(fn: ((ev: MessageEvent) => any) | null) {
            this._onmessage = fn;
            if (fn) {
              this.addEventListener('message', fn as EventListener);
            }
          },
          configurable: true,
        });
      }

      Object.defineProperty(window, '__sseLog', {
        get: () => _log,
        configurable: true,
      });
    });
  }

  /** Returns all captured SSE events of a given type. */
  eventsOfType(type: string): any[] {
    return (this.page as any).__sseEvents?.filter((e: any) => e.type === type) || [];
  }

  /** Waits for at least one event of the given type to arrive. */
  async waitForEvent(type: string, timeout = 10000): Promise<any> {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      const log: any[] = await this.page.evaluate(() => (window as any).__sseLog || []);
      const match = log.find((e: any) => e.type === type);
      if (match) return match.data;
      await this.page.waitForTimeout(100);
    }
    throw new Error(`SSE event "${type}" not received within ${timeout}ms`);
  }
}
