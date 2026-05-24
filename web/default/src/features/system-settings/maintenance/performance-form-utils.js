/**
 * @typedef {{
 *   'performance_setting.disk_cache_enabled': boolean
 *   'performance_setting.disk_cache_threshold_mb': number
 *   'performance_setting.disk_cache_max_size_mb': number
 *   'performance_setting.disk_cache_path': string
 *   'performance_setting.monitor_enabled': boolean
 *   'performance_setting.monitor_cpu_threshold': number
 *   'performance_setting.monitor_memory_threshold': number
 *   'performance_setting.monitor_disk_threshold': number
 *   'perf_metrics_setting.enabled': boolean
 *   'perf_metrics_setting.flush_interval': number
 *   'perf_metrics_setting.bucket_time': 'minute' | '5min' | 'hour'
 *   'perf_metrics_setting.retention_days': number
 * }} PerformanceBaseline
 */

/**
 * @typedef {{
 *   performance_setting: {
 *     disk_cache_enabled: boolean
 *     disk_cache_threshold_mb: number
 *     disk_cache_max_size_mb: number
 *     disk_cache_path?: string
 *     monitor_enabled: boolean
 *     monitor_cpu_threshold: number
 *     monitor_memory_threshold: number
 *     monitor_disk_threshold: number
 *   }
 *   perf_metrics_setting: {
 *     enabled: boolean
 *     flush_interval: number
 *     bucket_time: 'minute' | '5min' | 'hour'
 *     retention_days: number
 *   }
 * }} PerformanceFormValues
 */

/**
 * @param {PerformanceBaseline} defaults
 * @returns {PerformanceFormValues}
 */
export function buildPerformanceFormDefaults(defaults) {
  return {
    performance_setting: {
      disk_cache_enabled: defaults['performance_setting.disk_cache_enabled'],
      disk_cache_threshold_mb:
        defaults['performance_setting.disk_cache_threshold_mb'],
      disk_cache_max_size_mb:
        defaults['performance_setting.disk_cache_max_size_mb'],
      disk_cache_path: defaults['performance_setting.disk_cache_path'] ?? '',
      monitor_enabled: defaults['performance_setting.monitor_enabled'],
      monitor_cpu_threshold:
        defaults['performance_setting.monitor_cpu_threshold'],
      monitor_memory_threshold:
        defaults['performance_setting.monitor_memory_threshold'],
      monitor_disk_threshold:
        defaults['performance_setting.monitor_disk_threshold'],
    },
    perf_metrics_setting: {
      enabled: defaults['perf_metrics_setting.enabled'],
      flush_interval: defaults['perf_metrics_setting.flush_interval'],
      bucket_time: defaults['perf_metrics_setting.bucket_time'],
      retention_days: defaults['perf_metrics_setting.retention_days'],
    },
  }
}

/**
 * @param {PerformanceBaseline} values
 * @returns {PerformanceBaseline}
 */
export function normalizePerformanceBaseline(values) {
  return {
    'performance_setting.disk_cache_enabled': Boolean(
      values['performance_setting.disk_cache_enabled']
    ),
    'performance_setting.disk_cache_threshold_mb': Number(
      values['performance_setting.disk_cache_threshold_mb']
    ),
    'performance_setting.disk_cache_max_size_mb': Number(
      values['performance_setting.disk_cache_max_size_mb']
    ),
    'performance_setting.disk_cache_path': (
      values['performance_setting.disk_cache_path'] ?? ''
    ).trim(),
    'performance_setting.monitor_enabled': Boolean(
      values['performance_setting.monitor_enabled']
    ),
    'performance_setting.monitor_cpu_threshold': Number(
      values['performance_setting.monitor_cpu_threshold']
    ),
    'performance_setting.monitor_memory_threshold': Number(
      values['performance_setting.monitor_memory_threshold']
    ),
    'performance_setting.monitor_disk_threshold': Number(
      values['performance_setting.monitor_disk_threshold']
    ),
    'perf_metrics_setting.enabled': Boolean(
      values['perf_metrics_setting.enabled']
    ),
    'perf_metrics_setting.flush_interval': Number(
      values['perf_metrics_setting.flush_interval']
    ),
    'perf_metrics_setting.bucket_time':
      values['perf_metrics_setting.bucket_time'],
    'perf_metrics_setting.retention_days': Number(
      values['perf_metrics_setting.retention_days']
    ),
  }
}

/**
 * @param {PerformanceFormValues} values
 * @returns {PerformanceBaseline}
 */
export function normalizePerformanceFormValues(values) {
  return {
    'performance_setting.disk_cache_enabled': Boolean(
      values.performance_setting.disk_cache_enabled
    ),
    'performance_setting.disk_cache_threshold_mb': Number(
      values.performance_setting.disk_cache_threshold_mb
    ),
    'performance_setting.disk_cache_max_size_mb': Number(
      values.performance_setting.disk_cache_max_size_mb
    ),
    'performance_setting.disk_cache_path': (
      values.performance_setting.disk_cache_path ?? ''
    ).trim(),
    'performance_setting.monitor_enabled': Boolean(
      values.performance_setting.monitor_enabled
    ),
    'performance_setting.monitor_cpu_threshold': Number(
      values.performance_setting.monitor_cpu_threshold
    ),
    'performance_setting.monitor_memory_threshold': Number(
      values.performance_setting.monitor_memory_threshold
    ),
    'performance_setting.monitor_disk_threshold': Number(
      values.performance_setting.monitor_disk_threshold
    ),
    'perf_metrics_setting.enabled': Boolean(
      values.perf_metrics_setting.enabled
    ),
    'perf_metrics_setting.flush_interval': Number(
      values.perf_metrics_setting.flush_interval
    ),
    'perf_metrics_setting.bucket_time': values.perf_metrics_setting.bucket_time,
    'perf_metrics_setting.retention_days': Number(
      values.perf_metrics_setting.retention_days
    ),
  }
}

/**
 * @param {PerformanceFormValues} values
 * @param {PerformanceBaseline} baseline
 * @returns {Array<[keyof PerformanceBaseline, PerformanceBaseline[keyof PerformanceBaseline]]>}
 */
export function findChangedPerformanceUpdates(values, baseline) {
  const normalized = normalizePerformanceFormValues(values)
  return /** @type {Array<[keyof PerformanceBaseline, PerformanceBaseline[keyof PerformanceBaseline]]>} */ (
    Object.keys(normalized)
      .filter((key) => normalized[key] !== baseline[key])
      .map((key) => [key, normalized[key]])
  )
}
