// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
/**
 * Node unit tests for helpers.js 配置表单「代理列表」文本 ↔ 数组转换。
 * 背景：配置表单原绑定顶层 configForm.proxies，而 GET /api/config/server 返回
 * 嵌套的 downloader.proxies → 编辑被静默丢弃（预存 UI bug）。
 */
'use strict'

const { test } = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')

const window = {}
global.window = window
global.AppAPI = {}
global.Log = { info: function () {}, debug: function () {}, warn: function () {} }
global.document = { addEventListener: function () {} }

const src = fs.readFileSync(path.join(__dirname, 'helpers.js'), 'utf8')
;(new Function('window', src))(window)
const UiHelpers = window.UiHelpers

test('formatProxiesText: 数组 → 一行一个（过滤空项与非字符串）', () => {
  assert.strictEqual(UiHelpers.formatProxiesText(['http://a:1', 'http://b:2']), 'http://a:1\nhttp://b:2')
  assert.strictEqual(UiHelpers.formatProxiesText(['', 'http://a:1']), 'http://a:1')
  assert.strictEqual(UiHelpers.formatProxiesText(null), '')
  assert.strictEqual(UiHelpers.formatProxiesText(['x', 5]), 'x')
})

test('parseProxiesText: 多行文本 → 数组（trim + 去空行）', () => {
  assert.deepStrictEqual(UiHelpers.parseProxiesText('http://a:1\n\n  http://b:2  \n'), ['http://a:1', 'http://b:2'])
  assert.deepStrictEqual(UiHelpers.parseProxiesText(''), [])
  assert.deepStrictEqual(UiHelpers.parseProxiesText(null), [])
})

test('get/setConfigProxies: 读写 configForm.downloader.proxies（嵌套路径）', () => {
  const state = { configForm: { downloader: { proxies: ['http://a:1'] } } }
  assert.strictEqual(UiHelpers.getConfigProxies(state), 'http://a:1')

  UiHelpers.setConfigProxies(state, 'http://b:2\nhttp://c:3')
  assert.deepStrictEqual(state.configForm.downloader.proxies, ['http://b:2', 'http://c:3'])

  // 表单为空对象时自动补出 downloader 层级
  const empty = { configForm: {} }
  UiHelpers.setConfigProxies(empty, 'http://d:4')
  assert.deepStrictEqual(empty.configForm.downloader.proxies, ['http://d:4'])
  assert.strictEqual(UiHelpers.getConfigProxies({}), '')
})

// nl 用 fromCharCode 构造，避免转义序列在工具链中被解释为真换行
const nl = String.fromCharCode(10)

test('formatDomainLimits/parseDomainLimits: 域名限流文本 ↔ map 往返', () => {
  assert.strictEqual(UiHelpers.formatDomainLimits({ 'a.com': 2, 'b.com': 4 }), ['a.com=2', 'b.com=4'].join(nl))
  assert.strictEqual(UiHelpers.formatDomainLimits(null), '')

  const text = ['a.com=2', '', '  b.com=4  ', '# 注释', '非法行', 'c.com=0', 'd.com=x'].join(nl)
  assert.deepStrictEqual(UiHelpers.parseDomainLimits(text), { 'a.com': 2, 'b.com': 4 })
  assert.deepStrictEqual(UiHelpers.parseDomainLimits(null), {})
})

test('get/setConfigDomainLimits: 读写 configForm.downloader.domain_limits', () => {
  const state = { configForm: { downloader: { domain_limits: { 'a.com': 3 } } } }
  assert.strictEqual(UiHelpers.getConfigDomainLimits(state), 'a.com=3')

  UiHelpers.setConfigDomainLimits(state, 'b.com=5')
  assert.deepStrictEqual(state.configForm.downloader.domain_limits, { 'b.com': 5 })

  const empty = { configForm: {} }
  UiHelpers.setConfigDomainLimits(empty, 'c.com=7')
  assert.deepStrictEqual(empty.configForm.downloader.domain_limits, { 'c.com': 7 })
  assert.strictEqual(UiHelpers.getConfigDomainLimits({}), '')
})
