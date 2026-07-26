export interface PerformanceBaseline {
  'performance_setting.disk_cache_enabled': boolean
  'performance_setting.disk_cache_threshold_mb': number
  'performance_setting.disk_cache_max_size_mb': number
  'performance_setting.disk_cache_path': string
  'performance_setting.monitor_enabled': boolean
  'performance_setting.monitor_cpu_threshold': number
  'performance_setting.monitor_memory_threshold': number
  'performance_setting.monitor_disk_threshold': number
  'perf_metrics_setting.enabled': boolean
  'perf_metrics_setting.flush_interval': number
  'perf_metrics_setting.bucket_time': 'minute' | '5min' | 'hour'
  'perf_metrics_setting.retention_days': number
}

export interface PerformanceFormValues {
  performance_setting: {
    disk_cache_enabled: boolean
    disk_cache_threshold_mb: number
    disk_cache_max_size_mb: number
    disk_cache_path?: string
    monitor_enabled: boolean
    monitor_cpu_threshold: number
    monitor_memory_threshold: number
    monitor_disk_threshold: number
  }
  perf_metrics_setting: {
    enabled: boolean
    flush_interval: number
    bucket_time: 'minute' | '5min' | 'hour'
    retention_days: number
  }
}

export function buildPerformanceFormDefaults(
  defaults: PerformanceBaseline
): PerformanceFormValues
export function normalizePerformanceBaseline(
  values: PerformanceBaseline
): PerformanceBaseline
export function normalizePerformanceFormValues(
  values: PerformanceFormValues
): PerformanceBaseline
export function findChangedPerformanceUpdates(
  values: PerformanceFormValues,
  baseline: PerformanceBaseline
): Array<
  [
    keyof PerformanceBaseline,
    PerformanceBaseline[keyof PerformanceBaseline],
  ]
>
