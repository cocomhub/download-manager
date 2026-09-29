// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Node unit tests for retryPanel.js pure functions.
 * Run with: node --test web/static/app/ui/retryPanel.test.js
 * The module under test is IIFE-attached to globalThis; we load it via
 * eval of the file content with a window shim (AppAPI unused in pure fns).
 */
'use strict'

const { test } = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')

const src = fs.readFileSync(path.join(__dirname, 'retryPanel.js'), 'utf8')
const window = {}
window.AppAPI = {
  get: async () => ({ categories: {} }),
  post: async () => ({ ok: true })
}
;(new Function('window', src))(window)

const UiRetryPanel = window.UiRetryPanel

test('categoryLabel: known keys map to Chinese labels', () => {
  assert.strictEqual(UiRetryPanel.categoryLabel('http_4xx'), 'HTTP 4xx（永久）')
  assert.strictEqual(UiRetryPanel.categoryLabel('http_5xx'), 'HTTP 5xx（可重试）')
  assert.strictEqual(UiRetryPanel.categoryLabel('timeout'), '超时')
  assert.strictEqual(UiRetryPanel.categoryLabel('other'), '其它')
})

test('categoryLabel: unknown key falls back to key or 其它', () => {
  assert.strictEqual(UiRetryPanel.categoryLabel('bogus'), 'bogus')
  assert.strictEqual(UiRetryPanel.categoryLabel(''), '其它')
})

test('categoryColor: known keys return tailwind color', () => {
  assert.strictEqual(UiRetryPanel.categoryColor('http_4xx'), 'red')
  assert.strictEqual(UiRetryPanel.categoryColor('http_5xx'), 'orange')
  assert.strictEqual(UiRetryPanel.categoryColor('timeout'), 'yellow')
  assert.strictEqual(UiRetryPanel.categoryColor('other'), 'gray')
})

test('categoryColor: unknown key returns gray', () => {
  assert.strictEqual(UiRetryPanel.categoryColor('nope'), 'gray')
})

test('categoryCount: returns per-category count', () => {
  const overview = { categories: { http_4xx: 2, http_5xx: 3 } }
  assert.strictEqual(UiRetryPanel.categoryCount(overview, 'http_4xx'), 2)
  assert.strictEqual(UiRetryPanel.categoryCount(overview, 'timeout'), 0)
  assert.strictEqual(UiRetryPanel.categoryCount(null, 'http_4xx'), 0)
})

test('retryableCategories: excludes http_4xx, includes 5xx/timeout/other', () => {
  const overview = { categories: { http_4xx: 1, http_5xx: 1, timeout: 1, other: 1 } }
  const cats = UiRetryPanel.retryableCategories(overview)
  const keys = cats.map(function (c) { return c.key })
  assert.deepStrictEqual(keys, ['http_5xx', 'timeout', 'other'])
  assert.ok(keys.indexOf('http_4xx') === -1)
  assert.deepStrictEqual(UiRetryPanel.retryableCategories(null), [])
})
