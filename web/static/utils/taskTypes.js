/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function (global) {
  var TASK_TYPES = [
    { id: 'all', label: '全部' },
    { id: 'tktube', label: 'TKtube' },
    { id: 'vikacg', label: 'VikACG' },
    { id: 'hanime', label: 'Hanime' }
  ]
  function getAvailableTaskTypes() {
    return TASK_TYPES.slice()
  }
  global.getAvailableTaskTypes = getAvailableTaskTypes
  global.TASK_TYPES = TASK_TYPES
})(window)
