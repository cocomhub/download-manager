// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
/**
 * Node unit tests for taskList.js batch operations (P7-2).
 * Loads the IIFE module with a window shim; tests pure dispatch logic.
 */
'use strict'

const { test } = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')

// ---- shims ----
const calls = []
const window = {}
// IIFE 内部直接引用全局 AppAPI/UiHelpers/Log（非 window.*）→ 需在 new Function 的全局注入
// mock 使用真实 fetch Response 形状：{ ok, json: () => Promise.resolve(...) }
let mockJson = { 'http://x/1': 'ok', 'http://x/2': 'ok' }
global.AppAPI = {
  post: function (url, body) {
    calls.push(['post', url, body])
    return Promise.resolve({
      ok: true,
      json: function () { return Promise.resolve(mockJson) }
    })
  }
}
global.UiHelpers = {
  showToast: function (msg, type) { calls.push(['toast', msg, type]) }
}
global.Log = { info: function () {}, debug: function () {}, warn: function () {} }
global.fetchTaskDetails = function () {}
// 清理全局注入（避免测试间污染）
;['AppAPI', 'UiHelpers', 'Log', 'fetchTaskDetails'].forEach(function (k) {
  process.on('exit', function () { delete global[k] })
})
window.confirm = function () { return true }
// 加载模块
const src = fs.readFileSync(path.join(__dirname, 'taskList.js'), 'utf8')
;(new Function('window', src))(window)
const UiTaskList = window.UiTaskList

function toastCalls () {
  return calls.filter(function (c) { return c[0] === 'toast' })
}

test('deleteSelectedObjects: confirm + posts delete_batch with selected urls, toast counts ok', async () => {
  calls.length = 0
  mockJson = { 'http://x/1': 'ok', 'http://x/2': 'ok' }
  const state = {
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectedObjectUrls: ['http://x/1', 'http://x/2'],
    selectedTask: { objects: [] },
    selectAllScope: 'page',
    batchBusy: false
  }
  await UiTaskList.deleteSelectedObjects(state)
  const post = calls.find(function (c) { return c[0] === 'post' })
  assert.ok(post, 'should call AppAPI.post')
  assert.strictEqual(post[1], '/api/tasks/task-1/object/delete_batch')
  assert.deepStrictEqual(post[2], { urls: ['http://x/1', 'http://x/2'] })
  // 修复项 5：计数来自 res.json() 而非 res.data，toast 显示真实成功数。
  const toast = toastCalls().find(function (t) { return t[1].indexOf('已删除') === 0 })
  assert.ok(toast, 'should show delete toast')
  assert.strictEqual(toast[1], '已删除 2/2 个对象')
  assert.strictEqual(toast[2], 'success')
  assert.strictEqual(state.batchBusy, false, 'batchBusy should reset after completion')
})

test('deleteSelectedObjects: isWriteDisabled shows toast only, no API call', async () => {
  calls.length = 0
  const state = { isWriteDisabled: true, selectedObjectUrls: ['http://x/1'], selectedTaskId: 't', batchBusy: false }
  await UiTaskList.deleteSelectedObjects(state)
  const posts = calls.filter(function (c) { return c[0] === 'post' })
  assert.strictEqual(posts.length, 0, 'no API call when write disabled')
})

test('retrySelectedObjects: uses retry_batch for selected failed objects, toast counts ok', async () => {
  calls.length = 0
  mockJson = { 'http://x/1': 'ok', 'http://x/2': 'ok' }
  const state = {
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectAllScope: 'page',
    selectedObjectUrls: ['http://x/1', 'http://x/2'],
    batchBusy: false
  }
  await UiTaskList.retrySelectedObjects(state)
  const post = calls.find(function (c) { return c[0] === 'post' && c[1].indexOf('retry_batch') >= 0 })
  assert.ok(post, 'should call retry_batch')
  assert.deepStrictEqual(post[2], { urls: ['http://x/1', 'http://x/2'] })
  // 修复项 5：计数来自 res.json()，toast 显示真实成功数。
  const toast = toastCalls().find(function (t) { return t[1].indexOf('已重试') === 0 })
  assert.ok(toast, 'should show retry toast')
  assert.strictEqual(toast[1], '已重试 2/2 个对象')
  assert.strictEqual(toast[2], 'success')
  assert.strictEqual(state.batchBusy, false, 'batchBusy should reset after completion')
})

test('retrySelectedObjects: partial failure counts failed subset in toast', async () => {
  calls.length = 0
  mockJson = { 'http://x/1': 'ok', 'http://x/2': 'error' }
  const state = {
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectAllScope: 'page',
    selectedObjectUrls: ['http://x/1', 'http://x/2'],
    batchBusy: false
  }
  await UiTaskList.retrySelectedObjects(state)
  const toast = toastCalls().find(function (t) { return t[1].indexOf('已重试') === 0 })
  assert.ok(toast, 'should show retry toast')
  assert.strictEqual(toast[1], '已重试 1/2 个对象，失败 1 个')
  assert.strictEqual(toast[2], 'warning')
})

test('retrySelectedObjects: batchBusy prevents duplicate submit (in-flight guard)', async () => {
  calls.length = 0
  const state = {
    batchBusy: true,
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectAllScope: 'page',
    selectedObjectUrls: ['http://x/1']
  }
  await UiTaskList.retrySelectedObjects(state)
  const posts = calls.filter(function (c) { return c[0] === 'post' })
  assert.strictEqual(posts.length, 0, 'no API call while batchBusy is true')
})
