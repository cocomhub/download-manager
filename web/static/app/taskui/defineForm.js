// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * defineForm — 声明式表单工厂
 * 简单 task 类型通过配置字段列表快速生成 renderForm 函数。
 *
 * 支持字段类型：text, number, select, textarea, checkbox
 * 字段验证：required, min, max, pattern
 */
;(function () {
  'use strict'

  var FIELD_TYPES = {
    text: renderTextField,
    number: renderNumberField,
    select: renderSelectField,
    textarea: renderTextareaField,
    checkbox: renderCheckboxField,
  }

  /**
   * 创建一个声明式表单渲染函数
   * @param {Object} config
   * @param {Array} config.fields - 字段定义数组
   * @returns {Function} renderForm(h, formData, formErrors)
   */
  function defineForm(config) {
    config = config || {}
    var fields = config.fields || []

    return function renderForm(h, formData, formErrors) {
      formErrors = formErrors || {}
      return h('div', { class: 'space-y-4' }, fields.map(function (field) {
        var renderer = FIELD_TYPES[field.type] || renderTextField
        return renderer(h, field, formData, formErrors)
      }))
    }
  }

  function renderTextField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('input', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { type: 'text', placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { input: function (e) { data[field.key] = e.target.value } }
      })
    ])
  }

  function renderNumberField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('input', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { type: 'number', min: field.min, max: field.max, placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] !== undefined ? data[field.key] : field.default },
        on: { input: function (e) { data[field.key] = Number(e.target.value) } }
      })
    ])
  }

  function renderSelectField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('select', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { change: function (e) { data[field.key] = e.target.value } }
      }, (field.options || []).map(function (opt) {
        var optValue = typeof opt === 'string' ? opt : opt.value
        var optLabel = typeof opt === 'string' ? opt : opt.label
        return h('option', { attrs: { value: optValue } }, optLabel)
      }))
    ])
  }

  function renderTextareaField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('textarea', {
        class: 'w-full border border-gray-300 rounded-md p-2 font-mono text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { rows: field.rows || 5, placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { input: function (e) { data[field.key] = e.target.value } }
      })
    ])
  }

  function renderCheckboxField(h, field, data, errors) {
    return h('div', { class: 'flex items-center gap-2' }, [
      h('input', {
        attrs: { type: 'checkbox', id: 'field-' + field.key },
        domProps: { checked: !!data[field.key] },
        on: { change: function (e) { data[field.key] = e.target.checked } }
      }),
      h('label', { attrs: { for: 'field-' + field.key }, class: 'text-sm font-medium text-gray-700' }, field.label || '')
    ])
  }

  function renderFieldWrapper(h, field, errors, children) {
    var errorMsg = errors[field.key]
    return h('div', [
      field.label ? h('label', {
        attrs: { for: 'field-' + field.key },
        class: 'block text-sm font-medium text-gray-700 mb-1'
      }, field.label + (field.required ? ' *' : '')) : null,
      h('div', children),
      errorMsg ? h('p', { class: 'text-xs text-red-500 mt-1' }, errorMsg) : null,
      field.description ? h('p', { class: 'text-xs text-gray-500 mt-1' }, field.description) : null,
    ])
  }

  // 导出
  window.TaskUI = window.TaskUI || {}
  window.TaskUI.defineForm = defineForm
})()