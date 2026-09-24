// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Auth helpers — pure credential management, no Vue dependency.
 * Exposed as window.AuthHelper.
 *
 * Credential storage: localStorage key "dm.auth", value shape:
 *   { mode: "basic"|"token", username: string, password: string, token: string }
 * For basic mode, Authorization header = "Basic base64(username:password)".
 * For token mode, Authorization header = "Bearer <token>".
 */
;(function () {
  'use strict'

  var STORAGE_KEY = 'dm.auth'

  // base64Encode — Unicode-safe base64 (btoa fails on non-Latin1).
  function base64Encode(str) {
    // Use TextEncoder + chunked btoa to handle multi-byte chars.
    var bytes = new TextEncoder().encode(str)
    var binary = ''
    var chunkSize = 0x8000
    for (var i = 0; i < bytes.length; i += chunkSize) {
      binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunkSize))
    }
    return btoa(binary)
  }

  // getAuth returns the stored credential object or null.
  function getAuth() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return null
      var parsed = JSON.parse(raw)
      if (!parsed || typeof parsed !== 'object') return null
      if (parsed.mode !== 'basic' && parsed.mode !== 'token') return null
      return parsed
    } catch (e) {
      return null
    }
  }

  // setAuth stores a credential object.
  function setAuth(cred) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cred))
  }

  // clearAuth removes any stored credential.
  function clearAuth() {
    localStorage.removeItem(STORAGE_KEY)
  }

  // isAuthed returns true when a usable credential exists.
  function isAuthed() {
    var cred = getAuth()
    if (!cred) return false
    if (cred.mode === 'token') return !!cred.token
    return !!cred.username && !!cred.password
  }

  // authHeader returns the Authorization header value, or null when no credential.
  function authHeader() {
    var cred = getAuth()
    if (!cred) return null
    if (cred.mode === 'token') {
      return cred.token ? 'Bearer ' + cred.token : null
    }
    if (cred.username && cred.password) {
      return 'Basic ' + base64Encode(cred.username + ':' + cred.password)
    }
    return null
  }

  // verifyCredential sends a request with the given credential to the
  // verification endpoint. Resolves true on 200/204, false on 401, throws on network error.
  function verifyCredential(cred, fetchImpl) {
    var fetcher = fetchImpl || fetch
    var header = null
    if (cred.mode === 'token') {
      header = cred.token ? 'Bearer ' + cred.token : null
    } else {
      header = cred.username && cred.password
        ? 'Basic ' + base64Encode(cred.username + ':' + cred.password)
        : null
    }
    var opts = { method: 'GET', headers: {} }
    if (header) opts.headers.Authorization = header
    return fetcher('/api/auth/verify', opts).then(function (r) {
      if (r.status === 200 || r.status === 204) return true
      if (r.status === 401) return false
      // Other errors (500, network) — treat as "cannot verify" but not invalid creds.
      throw new Error('Auth verify failed with status ' + r.status)
    })
  }

  window.AuthHelper = {
    getAuth: getAuth,
    setAuth: setAuth,
    clearAuth: clearAuth,
    isAuthed: isAuthed,
    authHeader: authHeader,
    verifyCredential: verifyCredential,
    base64Encode: base64Encode,
    STORAGE_KEY: STORAGE_KEY
  }
})()
