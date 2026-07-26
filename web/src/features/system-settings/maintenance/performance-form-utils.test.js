import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildPerformanceFormDefaults,
  findChangedPerformanceUpdates,
} from './performance-form-utils.js'

test('detects monitor toggle changes from nested performance form values', () => {
  const defaultValues = {
    'performance_setting.disk_cache_enabled': false,
    'performance_setting.disk_cache_threshold_mb': 10,
    'performance_setting.disk_cache_max_size_mb': 1024,
    'performance_setting.disk_cache_path': '',
    'performance_setting.monitor_enabled': false,
    'performance_setting.monitor_cpu_threshold': 90,
    'performance_setting.monitor_memory_threshold': 90,
    'performance_setting.monitor_disk_threshold': 95,
    'perf_metrics_setting.enabled': true,
    'perf_metrics_setting.flush_interval': 5,
    'perf_metrics_setting.bucket_time': 'hour',
    'perf_metrics_setting.retention_days': 0,
  }

  const formValues = buildPerformanceFormDefaults(defaultValues)
  formValues.performance_setting.monitor_enabled = true

  assert.deepEqual(findChangedPerformanceUpdates(formValues, defaultValues), [
    ['performance_setting.monitor_enabled', true],
  ])
})
