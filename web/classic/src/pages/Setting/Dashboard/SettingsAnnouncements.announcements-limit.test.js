import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import {
  ANNOUNCEMENT_CONTENT_LIMIT,
  ANNOUNCEMENT_CONTENT_LIMIT_ERROR,
  getAnnouncementContentLength,
  validateAnnouncementContentBeforeSave,
} from './SettingsAnnouncements.validation.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(
  join(__dirname, 'SettingsAnnouncements.jsx'),
  'utf8'
)

test('classic announcement local save validation accepts 3000 Chinese characters', () => {
  assert.equal(
    validateAnnouncementContentBeforeSave('测'.repeat(ANNOUNCEMENT_CONTENT_LIMIT)),
    null
  )
})

test('classic announcement local save validation rejects 3001 Chinese characters', () => {
  assert.equal(
    validateAnnouncementContentBeforeSave('测'.repeat(ANNOUNCEMENT_CONTENT_LIMIT + 1)),
    ANNOUNCEMENT_CONTENT_LIMIT_ERROR
  )
})

test('classic announcement local save validation accepts 3000 emoji code points', () => {
  assert.equal(
    getAnnouncementContentLength('😀'.repeat(ANNOUNCEMENT_CONTENT_LIMIT)),
    ANNOUNCEMENT_CONTENT_LIMIT
  )
  assert.equal(
    validateAnnouncementContentBeforeSave('😀'.repeat(ANNOUNCEMENT_CONTENT_LIMIT)),
    null
  )
})

test('classic announcement local save validation rejects 3001 emoji code points', () => {
  assert.equal(
    validateAnnouncementContentBeforeSave('😀'.repeat(ANNOUNCEMENT_CONTENT_LIMIT + 1)),
    ANNOUNCEMENT_CONTENT_LIMIT_ERROR
  )
})

test('classic announcement editor no longer relies on maxCount for Unicode length enforcement', () => {
  assert.doesNotMatch(source, /maxCount=\{ANNOUNCEMENT_CONTENT_LIMIT\}/)
})
