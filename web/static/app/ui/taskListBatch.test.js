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
global.AppAPI = {
  post: function (url, body) {
    calls.push(['post', url, body])
    return Promise.resolve({ ok: true, data: { 'http://x/1': 'ok', 'http://x/2': 'ok' } })
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

test('deleteSelectedObjects: confirm + posts delete_batch with selected urls', async () => {
  calls.length = 0
  const state = {
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectedObjectUrls: ['http://x/1', 'http://x/2'],
    selectedTask: { objects: [] },
    selectedObjectUrls: [],
    selectAllScope: 'page'
  }
  state.selectedObjectUrls = ['http://x/1', 'http://x/2']
  await UiTaskList.deleteSelectedObjects(state)
  const post = calls.find(function (c) { return c[0] === 'post' })
  assert.ok(post, 'should call AppAPI.post')
  assert.strictEqual(post[1], '/api/tasks/task-1/object/delete_batch')
  assert.deepStrictEqual(post[2], { urls: ['http://x/1', 'http://x/2'] })
})

test('deleteSelectedObjects: isWriteDisabled shows toast only, no API call', async () => {
  calls.length = 0
  const state = { isWriteDisabled: true, selectedObjectUrls: ['http://x/1'], selectedTaskId: 't' }
  await UiTaskList.deleteSelectedObjects(state)
  const posts = calls.filter(function (c) { return c[0] === 'post' })
  assert.strictEqual(posts.length, 0, 'no API call when write disabled')
})

test('retrySelectedObjects: uses retry_batch for selected failed objects', async () => {
  calls.length = 0
  const state = {
    isWriteDisabled: false,
    selectedTaskId: 'task-1',
    selectAllScope: 'page',
    selectedObjectUrls: ['http://x/1', 'http://x/2']
  }
  await UiTaskList.retrySelectedObjects(state)
  const post = calls.find(function (c) { return c[0] === 'post' && c[1].indexOf('retry_batch') >= 0 })
  assert.ok(post, 'should call retry_batch')
  assert.deepStrictEqual(post[2], { urls: ['http://x/1', 'http://x/2'] })
})
