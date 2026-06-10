// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Dashboard methods — health check, metrics, failure records with polling.
 * Registered as Vue methods on the app.
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

  window.AppDashboard = {
    register: function (app) {
      app.mixin({methods: {
        // --- Dashboard data fetching ---

        fetchDashboardData: function () {
          this.fetchHealthz()
          this.fetchMetrics()
          this.fetchFailures()
        },

        fetchHealthz: function () {
          var self = this
          AppAPI.healthz().then(function (data) {
            self.dashboardHealth = data
          }).catch(function (e) {
            console.error('Dashboard healthz error:', e)
            self.dashboardHealth = { status: 'error', components: {} }
          })
        },

        fetchMetrics: function () {
          var self = this
          AppAPI.metrics().then(function (data) {
            self.dashboardMetrics = data
          }).catch(function (e) {
            console.error('Dashboard metrics error:', e)
          })
        },

        fetchFailures: function () {
          var self = this
          var limit = self.dashboardFailuresLimit || 20
          var taskId = self.dashboardFailuresTaskId || ''
          AppAPI.failures({ limit: limit, task_id: taskId }).then(function (data) {
            self.dashboardFailures = data
          }).catch(function (e) {
            console.error('Dashboard failures error:', e)
          })
        },

        // --- Polling timer management ---

        startDashboardPolling: function () {
          var self = this
          this.stopDashboardPolling() // clear any existing timers

          // Healthz: 5s interval
          this.dashboardHealthzTimer = setInterval(function () {
            self.fetchHealthz()
          }, 5000)

          // Metrics: 10s interval
          this.dashboardMetricsTimer = setInterval(function () {
            self.fetchMetrics()
          }, 10000)

          // Failures: 15s interval
          this.dashboardFailuresTimer = setInterval(function () {
            self.fetchFailures()
          }, 15000)
        },

        stopDashboardPolling: function () {
          if (this.dashboardHealthzTimer) {
            clearInterval(this.dashboardHealthzTimer)
            this.dashboardHealthzTimer = null
          }
          if (this.dashboardMetricsTimer) {
            clearInterval(this.dashboardMetricsTimer)
            this.dashboardMetricsTimer = null
          }
          if (this.dashboardFailuresTimer) {
            clearInterval(this.dashboardFailuresTimer)
            this.dashboardFailuresTimer = null
          }
        },

        // --- Dashboard failure filter ---

        changeDashboardFailuresLimit: function () {
          this.fetchFailures()
        },

        searchDashboardFailures: function () {
          this.fetchFailures()
        }
      }})
    }
  }
})()