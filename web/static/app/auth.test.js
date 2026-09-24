// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Node unit tests for auth.js pure functions.
 * Run with: node --test web/static/app/auth.test.js
 * The module under test is IIFE-attached to globalThis; we load it via
 * eval of the file content so localStorage can be stubbed.
 */
'use strict'

const { test } = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')

// Stub localStorage
const store = {}
global.localStorage = {
  getItem: (k) => (k in store ? store[k] : null),
  setItem: (k, v) => { store[k] = String(v) },
  removeItem: (k) => { delete store[k] }
}

// Minimal btoa/TextEncoder for Node (Node 18+ has TextEncoder; btoa is global since 16)
const src = fs.readFileSync(path.join(__dirname, 'auth.js'), 'utf8')
// Execute the IIFE in this context
;(new Function('window', src))(globalThis)

const AuthHelper = globalThis.AuthHelper

test('authHeader: basic mode encodes username:password as Base64', () => {
  AuthHelper.clearAuth()
  AuthHelper.setAuth({ mode: 'basic', username: 'admin', password: 'secret' })
  const header = AuthHelper.authHeader()
  assert.ok(header.startsWith('Basic '))
  const decoded = Buffer.from(header.slice(6), 'base64').toString('utf8')
  assert.strictEqual(decoded, 'admin:secret')
})

test('authHeader: token mode uses Bearer prefix', () => {
  AuthHelper.clearAuth()
  AuthHelper.setAuth({ mode: 'token', token: 'abc123' })
  const header = AuthHelper.authHeader()
  assert.strictEqual(header, 'Bearer abc123')
})

test('authHeader: no credential returns null', () => {
  AuthHelper.clearAuth()
  assert.strictEqual(AuthHelper.authHeader(), null)
})

test('isAuthed: basic requires username+password', () => {
  AuthHelper.clearAuth()
  assert.strictEqual(AuthHelper.isAuthed(), false)
  AuthHelper.setAuth({ mode: 'basic', username: 'u', password: '' })
  assert.strictEqual(AuthHelper.isAuthed(), false)
  AuthHelper.setAuth({ mode: 'basic', username: 'u', password: 'p' })
  assert.strictEqual(AuthHelper.isAuthed(), true)
})

test('isAuthed: token requires non-empty token', () => {
  AuthHelper.clearAuth()
  AuthHelper.setAuth({ mode: 'token', token: '' })
  assert.strictEqual(AuthHelper.isAuthed(), false)
  AuthHelper.setAuth({ mode: 'token', token: 't' })
  assert.strictEqual(AuthHelper.isAuthed(), true)
})

test('getAuth: returns null for invalid stored data', () => {
  store['dm.auth'] = 'not-json'
  assert.strictEqual(AuthHelper.getAuth(), null)
  store['dm.auth'] = JSON.stringify({ mode: 'weird' })
  assert.strictEqual(AuthHelper.getAuth(), null)
  AuthHelper.clearAuth()
})

test('verifyCredential: 200 resolves true, 401 resolves false', async () => {
  const fake200 = async () => ({ status: 200 })
  const fake401 = async () => ({ status: 401 })
  AuthHelper.clearAuth()
  assert.strictEqual(await AuthHelper.verifyCredential({ mode: 'basic', username: 'u', password: 'p' }, fake200), true)
  assert.strictEqual(await AuthHelper.verifyCredential({ mode: 'basic', username: 'u', password: 'p' }, fake401), false)
})

test('verifyCredential: 500 throws', async () => {
  const fake500 = async () => ({ status: 500 })
  AuthHelper.clearAuth()
  await assert.rejects(() => AuthHelper.verifyCredential({ mode: 'basic', username: 'u', password: 'p' }, fake500))
})
