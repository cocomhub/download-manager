// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Logger — 前端分级日志系统
 *
 * 级别（从低到高）：trace, debug, info, warn, error
 * 默认不开启（level = ''），通过服务端 runtime.log_level 配置控制。
 * 日志级别在 initRuntime() 时从 /api/runtime 获取并设置。
 *
 * 用法：
 *   Log.trace('message', { data: 1 });
 *   Log.debug('message');
 *   Log.info('message');
 *   Log.warn('message');
 *   Log.error('message');
 *   Log.setLevel('debug');    // 运行时动态切换
 *   Log.getLevel();           // 返回当前级别
 */
;(function () {
  'use strict'

  var LEVELS = {
    trace: 0,
    debug: 1,
    info: 2,
    warn: 3,
    error: 4,
  }

  var currentLevel = '' // 空字符串 = 不输出任何日志

  var LEVEL_NAMES = ['TRACE', 'DEBUG', 'INFO', 'WARN', 'ERROR']

  function shouldLog(level) {
    if (!currentLevel) return false
    var min = LEVELS[currentLevel]
    if (min === undefined) return false
    return LEVELS[level] >= min
  }

  function formatTime() {
    var d = new Date()
    return d.getHours().toString().padStart(2, '0') + ':' +
      d.getMinutes().toString().padStart(2, '0') + ':' +
      d.getSeconds().toString().padStart(2, '0') + '.' +
      d.getMilliseconds().toString().padStart(3, '0')
  }

  function log(level, msg, data) {
    if (!shouldLog(level)) return
    var prefix = '[' + formatTime() + '][' + level.toUpperCase() + ']'
    var idx = LEVELS[level]
    switch (idx) {
      case 0: // trace
      case 1: // debug
        if (data !== undefined) console.debug(prefix, msg, data)
        else console.debug(prefix, msg)
        break
      case 2: // info
        if (data !== undefined) console.log(prefix, msg, data)
        else console.log(prefix, msg)
        break
      case 3: // warn
        if (data !== undefined) console.warn(prefix, msg, data)
        else console.warn(prefix, msg)
        break
      case 4: // error
        if (data !== undefined) console.error(prefix, msg, data)
        else console.error(prefix, msg)
        break
    }
  }

  window.Log = {
    trace: function (msg, data) { log('trace', msg, data) },
    debug: function (msg, data) { log('debug', msg, data) },
    info: function (msg, data) { log('info', msg, data) },
    warn: function (msg, data) { log('warn', msg, data) },
    error: function (msg, data) { log('error', msg, data) },

    // 设置日志级别。空字符串或 'off' 关闭所有日志。
    setLevel: function (level) {
      if (!level || level === 'off') {
        currentLevel = ''
        return
      }
      if (LEVELS[level] !== undefined) {
        currentLevel = level
        console.debug('[Logger] log level set to', level)
      }
    },

    getLevel: function () {
      return currentLevel || 'off'
    }
  }
})()