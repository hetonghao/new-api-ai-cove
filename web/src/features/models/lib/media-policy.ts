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
import { z } from 'zod'

export type MediaModelType = 'image' | 'video'

export interface MediaParameter {
  type: 'string' | 'integer'
  enum?: string[]
  minimum?: number
  maximum?: number
}

export interface MediaReference {
  input: 'none' | 'inline' | 'url'
  max_images: number
}

export interface MediaOperation {
  protocol: string
  path: string
  parameters: Record<string, MediaParameter>
  reference: MediaReference
}

export interface MediaModelProfile {
  id: string
  type: MediaModelType
  operations: Record<string, MediaOperation>
  selection_hint?: string
}

export interface MediaPolicy {
  version: number
  models: MediaModelProfile[]
  image_priority: string[]
  video_priority: string[]
}

export interface MediaPolicySnapshot {
  config_version: string
  policy: MediaPolicy
}

export interface GetMediaModelsPolicyResponse {
  success: boolean
  message?: string
  data?: MediaPolicySnapshot
}

export interface UpdateMediaModelsPolicyResponse {
  success: boolean
  message?: string
  data?: MediaPolicySnapshot
}

export const mediaPolicyDraftSchema = z.object({
  imagePriority: z.string(),
  videoPriority: z.string(),
  modelsJson: z.string(),
})

export type MediaPolicyDraftValues = z.infer<typeof mediaPolicyDraftSchema>

const MEDIA_OPERATION_PATHS: Record<string, Record<string, string>> = {
  openai_images: {
    text_to_image: '/v1/images/generations',
    image_to_image: '/v1/images/edits',
  },
  gemini_generate_content: {
    text_to_image: '/v1beta/models/{model}:generateContent',
    image_to_image: '/v1beta/models/{model}:generateContent',
  },
  openai_video: {
    text_to_video: '/v1/videos',
    image_to_video: '/v1/videos',
  },
}

const MEDIA_TYPE_OPERATIONS: Record<MediaModelType, string[]> = {
  image: ['text_to_image', 'image_to_image'],
  video: ['text_to_video', 'image_to_video'],
}

export function prioritiesFromText(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
}

export function prioritiesToText(ids: string[]): string {
  return ids.join('\n')
}

export function mediaPolicyToDraft(
  policy: MediaPolicy
): MediaPolicyDraftValues {
  return {
    imagePriority: prioritiesToText(policy.image_priority),
    videoPriority: prioritiesToText(policy.video_priority),
    modelsJson: JSON.stringify(policy.models, null, 2),
  }
}

export type MediaDraftIssueField =
  | 'modelsJson'
  | 'imagePriority'
  | 'videoPriority'

export type MediaDraftIssueCode =
  | 'invalid_json'
  | 'models_not_array'
  | 'malformed_profile'
  | 'selection_hint_too_long'
  | 'duplicate_priority'
  | 'unknown_priority'
  | 'priority_type_mismatch'

export interface MediaDraftIssue {
  field: MediaDraftIssueField
  code: MediaDraftIssueCode
  detail?: string
}

export interface MediaDraftValidation {
  policy?: MediaPolicy
  models?: MediaModelProfile[]
  issues: MediaDraftIssue[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isValidModelID(id: unknown): id is string {
  return (
    typeof id === 'string' &&
    id.length > 0 &&
    new TextEncoder().encode(id).length <= 128 &&
    !/[\s?#%\\]/u.test(id) &&
    [...id].every((character) => {
      const code = character.codePointAt(0) ?? 0
      return code >= 32 && code !== 127
    })
  )
}

function isValidMediaOperation(
  name: string,
  type: MediaModelType,
  operation: unknown
): operation is MediaOperation {
  if (!MEDIA_TYPE_OPERATIONS[type].includes(name)) return false
  if (!isRecord(operation)) return false
  const protocol = operation.protocol
  if (typeof protocol !== 'string') return false
  const paths = MEDIA_OPERATION_PATHS[protocol]
  if (!paths || paths[name] !== operation.path) return false
  if (!isRecord(operation.parameters)) return false
  const allowedParameters: Record<string, string[]> = {
    openai_images: ['size', 'quality', 'n', 'response_format'],
    gemini_generate_content: ['aspect_ratio', 'resolution'],
    openai_video: ['seconds', 'size', 'aspect_ratio', 'resolution'],
  }
  for (const [name, parameter] of Object.entries(operation.parameters)) {
    if (!allowedParameters[protocol]?.includes(name) || !isRecord(parameter)) {
      return false
    }
    const expectedType =
      name === 'n' || name === 'seconds' ? 'integer' : 'string'
    if (parameter.type !== expectedType) return false
    if (parameter.type === 'integer') {
      if (
        typeof parameter.minimum !== 'number' ||
        typeof parameter.maximum !== 'number' ||
        !Number.isInteger(parameter.minimum) ||
        !Number.isInteger(parameter.maximum) ||
        parameter.minimum < 1 ||
        parameter.minimum > parameter.maximum
      ) {
        return false
      }
    }
    if (parameter.enum !== undefined) {
      if (
        !Array.isArray(parameter.enum) ||
        parameter.enum.some((entry) => typeof entry !== 'string')
      ) {
        return false
      }
    }
  }
  const reference = operation.reference
  if (!isRecord(reference)) return false
  if (
    reference.input !== 'none' &&
    reference.input !== 'inline' &&
    reference.input !== 'url'
  ) {
    return false
  }
  if (
    typeof reference.max_images !== 'number' ||
    !Number.isInteger(reference.max_images) ||
    reference.max_images < 0 ||
    reference.max_images > 16
  ) {
    return false
  }
  const textOperation = name === 'text_to_image' || name === 'text_to_video'
  if (
    textOperation &&
    (reference.input !== 'none' || reference.max_images !== 0)
  ) {
    return false
  }
  if (
    !textOperation &&
    (reference.max_images < 1 || reference.input === 'none')
  ) {
    return false
  }
  if (!textOperation && type === 'video' && reference.max_images !== 1) {
    return false
  }
  if (
    !textOperation &&
    protocol === 'gemini_generate_content' &&
    reference.input !== 'inline'
  ) {
    return false
  }
  return true
}

export function parseMediaModelsJson(raw: string): {
  models?: MediaModelProfile[]
  issue?: MediaDraftIssue
} {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return { issue: { field: 'modelsJson', code: 'invalid_json' } }
  }
  if (!Array.isArray(parsed)) {
    return { issue: { field: 'modelsJson', code: 'models_not_array' } }
  }
  const seen = new Set<string>()
  let hintIssue: MediaDraftIssue | undefined
  for (const entry of parsed) {
    if (!isRecord(entry) || !isValidModelID(entry.id)) {
      return { issue: { field: 'modelsJson', code: 'malformed_profile' } }
    }
    if (seen.has(entry.id)) {
      return {
        issue: {
          field: 'modelsJson',
          code: 'malformed_profile',
          detail: entry.id,
        },
      }
    }
    seen.add(entry.id)
    if (entry.type !== 'image' && entry.type !== 'video') {
      return { issue: { field: 'modelsJson', code: 'malformed_profile' } }
    }
    const operations = entry.operations
    if (!isRecord(operations) || Object.keys(operations).length === 0) {
      return { issue: { field: 'modelsJson', code: 'malformed_profile' } }
    }
    const type = entry.type
    for (const [name, operation] of Object.entries(operations)) {
      if (!isValidMediaOperation(name, type, operation)) {
        return {
          issue: {
            field: 'modelsJson',
            code: 'malformed_profile',
            detail: entry.id,
          },
        }
      }
    }
    const hint = entry.selection_hint
    if (hint !== undefined) {
      if (typeof hint !== 'string') {
        return {
          issue: {
            field: 'modelsJson',
            code: 'malformed_profile',
            detail: entry.id,
          },
        }
      }
      if (!hintIssue && [...hint].length > 2000) {
        hintIssue = {
          field: 'modelsJson',
          code: 'selection_hint_too_long',
          detail: entry.id,
        }
      }
    }
  }
  return { models: parsed as MediaModelProfile[], issue: hintIssue }
}

export function validateMediaPolicyDraft(
  values: MediaPolicyDraftValues
): MediaDraftValidation {
  const issues: MediaDraftIssue[] = []
  const { models, issue } = parseMediaModelsJson(values.modelsJson)
  if (issue) issues.push(issue)

  const registry = new Map<string, MediaModelType>()
  for (const profile of models ?? []) {
    registry.set(profile.id, profile.type)
  }

  const checkPriority = (
    raw: string,
    field: 'imagePriority' | 'videoPriority',
    type: MediaModelType
  ): string[] => {
    const ids = prioritiesFromText(raw)
    const seen = new Set<string>()
    for (const id of ids) {
      if (seen.has(id)) {
        issues.push({ field, code: 'duplicate_priority', detail: id })
        continue
      }
      seen.add(id)
      if (models === undefined) continue
      const profileType = registry.get(id)
      if (profileType === undefined) {
        issues.push({ field, code: 'unknown_priority', detail: id })
      } else if (profileType !== type) {
        issues.push({ field, code: 'priority_type_mismatch', detail: id })
      }
    }
    return ids
  }

  const imagePriority = checkPriority(
    values.imagePriority,
    'imagePriority',
    'image'
  )
  const videoPriority = checkPriority(
    values.videoPriority,
    'videoPriority',
    'video'
  )

  if (issues.length > 0 || models === undefined) {
    return { issues }
  }
  return {
    issues,
    models,
    policy: {
      version: 1,
      models,
      image_priority: imagePriority,
      video_priority: videoPriority,
    },
  }
}
