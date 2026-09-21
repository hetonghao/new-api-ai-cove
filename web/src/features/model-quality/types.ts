/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data?: T
}

export interface QualityConfig {
  model: string
  output_type: 'svg' | 'text'
  mode: 'route' | 'channel'
  protocol: 'responses' | 'chat'
  token_id: number
  group: string
  channel_ids: number[]
  prompt: string
  instruction: string
  instruction_role: 'system' | 'developer' | ''
  max_output_tokens: number
  reasoning_effort: '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'
  temperature?: number
  top_p?: number
  samples_per_target: number
  timeout_seconds: number
  daily_limit: number
}

export interface QualitySchedule {
  enabled: boolean
  kind: 'interval' | 'daily' | 'weekly'
  interval_minutes: number
  time: string
  weekdays: number[]
  timezone: string
}

export interface QualityCaseView {
  id: number
  name: string
  description: string
  enabled: boolean
  archived: boolean
  version: number
  edit_version: number
  schedule_version: number
  next_run_at: number
  last_schedule_reason: string
  active_run_id: string
  created_by: number
  updated_by: number
  created_at: number
  updated_at: number
  config: QualityConfig
  schedule: QualitySchedule
}

export interface QualityCaseWrite {
  name: string
  description: string
  enabled: boolean
  expected_edit_version: number
  config: QualityConfig
  schedule: QualitySchedule
}

export interface QualityRunRequest {
  version: number
  channel_ids?: number[]
  samples_per_target?: number
}

export interface ModelQualityRun {
  id: string
  case_id: number
  version: number
  source: string
  status: 'queued' | 'running' | 'completed' | 'cancelled' | string
  cancel_requested: boolean
  created_by: number
  created_at: number
  started_at: number
  finished_at: number
  sample_count: number
  executor_id: string
}

export type QualitySampleStatus =
  | 'pending'
  | 'requesting'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
  | 'skipped'
  | (string & {})

export interface ModelQualitySample {
  id: number
  run_id: string
  ordinal: number
  case_id: number
  version: number
  target_channel_id: number
  channel_id: number
  channel_name: string
  model: string
  response_model: string
  output_type: 'svg' | 'text' | string
  source: string
  status: QualitySampleStatus
  request_success: boolean
  request_id: string
  started_at: number
  created_at: number
  finished_at: number
  duration_ms: number | null
  first_text_ms: number | null
  input_tokens: number | null
  output_tokens: number | null
  error_code: string
  validation: string
  finish_reason: string
  annotation: string
  note: string
  annotated_by: number
  annotated_at: number
  artifact_expired: boolean
  pinned: boolean
}

export interface QualityChannelView {
  id: number
  name: string
}

export interface QualityBucket {
  start: number
  end: number
  success: number
  failure: number
  latest_at: number
}

export interface QualitySummary {
  success: number
  failure: number
  pending: number
  running: number
  cancelled: number
  interrupted: number
  skipped: number
  total: number
  success_rate: number | null
  p50_ms: number | null
  p95_ms: number | null
}

export interface QualityDashboardData {
  case_id: number
  version: number
  channel_id: number
  channels: QualityChannelView[]
  as_of: number
  window_start: number
  window_end: number
  summary: QualitySummary
  buckets: QualityBucket[]
}

export interface QualityCapabilities {
  channels: {
    id: number
    name: string
    models: string[]
    groups: string[]
  }[]
  tokens: { id: number; name: string; group: string }[]
  can_operate: boolean
  can_configure: boolean
}

export interface QualityArtifact {
  sample_id: number
  text: string
  svg: string
  sha256: string
  validation_version: string
  expired: boolean
}

export interface QualitySettingsConfig {
  enabled: boolean
  token_ids: number[]
  daily_limit: number
  concurrency: number
  retention_enabled: boolean
  artifact_days: number
  metadata_days: number
}

export interface QualitySettingsResponse {
  config: QualitySettingsConfig
  version: number
}
