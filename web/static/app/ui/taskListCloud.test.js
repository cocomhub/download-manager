// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
/**
 * Node unit tests for taskList.js 下载项「云端下载」选项开关。
 * 加载 IIFE 模块并注入 window/global shim；测试纯 dispatch 逻辑。
 */
'use strict'

const { test } = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')

const calls = []
const window = {}
let mockOK = true
global.AppAPI = {
  post: function (url, body) {
    calls.push(['post', url, body])
    return Promise.resolve({ ok: mockOK, json: function () { return Promise.resolve({}) } })
  }
}
global.UiHelpers = {
  showToast: function (msg, type) { calls.push(['toast', msg, type]) }
}
global.Log = { info: function () {}, debug: function () {}, warn: function () {} }
global.fetchTaskDetails = function () {}
;['AppAPI', 'UiHelpers', 'Log', 'fetchTaskDetails'].forEach(function (k) {
  process.on('exit', function () { delete global[k] })
})
window.confirm = function () { return true }

const src = fs.readFileSync(path.join(__dirname, 'taskList.js'), 'utf8')
;(new Function('window', src))(window)
const UiTaskList = window.UiTaskList

function newState (obj) {
  return { isWriteDisabled: false, selectedTaskId: 'task-1', selectedTask: { objects: [obj] } }
}

test('toggleObjectCloudDownload: 关闭态点击 → 提交 enabled=true 并置位', async () => {
  calls.length = 0
  mockOK = true
  const obj = { url: 'http://x/1' }
  await UiTaskList.toggleObjectCloudDownload(newState(obj), obj)
  const post = calls.find(function (c) { return c[0] === 'post' })
  assert.ok(post, 'should POST')
  assert.strictEqual(post[1], '/api/tasks/task-1/object/cloud_download')
  assert.deepStrictEqual(post[2], { url: 'http://x/1', enabled: true })
  assert.strictEqual(obj.cloud_download, true)
})

test('toggleObjectCloudDownload: 开启态点击 → 提交 enabled=false 并清位', async () => {
  calls.length = 0
  mockOK = true
  const obj = { url: 'http://x/2', cloud_download: true }
  await UiTaskList.toggleObjectCloudDownload(newState(obj), obj)
  const post = calls.find(function (c) { return c[0] === 'post' })
  assert.deepStrictEqual(post[2], { url: 'http://x/2', enabled: false })
  assert.strictEqual(obj.cloud_download, false)
})

test('toggleObjectCloudDownload: UI-Only 写禁用 → 不提交仅提示', async () => {
  calls.length = 0
  const st = newState({ url: 'http://x/3' })
  st.isWriteDisabled = true
  await UiTaskList.toggleObjectCloudDownload(st, st.selectedTask.objects[0])
  assert.strictEqual(calls.filter(function (c) { return c[0] === 'post' }).length, 0)
  assert.ok(calls.some(function (c) { return c[0] === 'toast' && c[2] === 'error' }))
})

test('toggleObjectCloudDownload: 服务端失败 → 不置位并提示错误', async () => {
  calls.length = 0
  mockOK = false
  const obj = { url: 'http://x/4' }
  await UiTaskList.toggleObjectCloudDownload(newState(obj), obj)
  assert.strictEqual(obj.cloud_download, undefined)
  assert.ok(calls.some(function (c) { return c[0] === 'toast' && c[2] === 'error' }))
})

test('toggleObjectCloudDownload: 并发点击只提交一次（in-flight 防重）', async () => {
  calls.length = 0
  let resolvePost = null
  const origPost = global.AppAPI.post
  global.AppAPI.post = function (url, body) {
    calls.push(['post', url, body])
    return new Promise(function (resolve) {
      resolvePost = function () { resolve({ ok: true, json: function () { return Promise.resolve({}) } }) }
    })
  }
  const obj = { url: 'http://x/5' }
  const st = newState(obj)
  const p1 = UiTaskList.toggleObjectCloudDownload(st, obj)
  const p2 = UiTaskList.toggleObjectCloudDownload(st, obj)
  assert.strictEqual(calls.filter(function (c) { return c[0] === 'post' }).length, 1)
  resolvePost()
  await Promise.all([p1, p2])
  global.AppAPI.post = origPost
  assert.strictEqual(obj.cloud_download, true)
})
