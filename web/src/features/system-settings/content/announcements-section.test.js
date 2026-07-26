import assert from 'node:assert/strict'
import test from 'node:test'

import {
  ANNOUNCEMENT_CONTENT_LIMIT,
  announcementSchema,
  ANNOUNCEMENT_CONTENT_DESCRIPTION,
  ANNOUNCEMENT_CONTENT_LIMIT_ERROR,
  getAnnouncementContentLength,
} from './announcements-section.validation.ts'

test('default announcements schema accepts 3000 Chinese characters', () => {
  const parsed = announcementSchema.parse({
    content: '测'.repeat(ANNOUNCEMENT_CONTENT_LIMIT),
    publishDate: '2026-06-16T12:00:00.000Z',
    type: 'default',
    extra: '',
  })

  assert.equal(parsed.content.length, ANNOUNCEMENT_CONTENT_LIMIT)
  assert.equal(
    ANNOUNCEMENT_CONTENT_DESCRIPTION,
    'Maximum 3000 characters (counted by Unicode characters). Supports Markdown and HTML.'
  )
})

test('default announcements schema rejects 3001 Chinese characters', () => {
  const result = announcementSchema.safeParse({
    content: '测'.repeat(ANNOUNCEMENT_CONTENT_LIMIT + 1),
    publishDate: '2026-06-16T12:00:00.000Z',
    type: 'default',
    extra: '',
  })

  assert.equal(result.success, false)
  assert.equal(result.error.issues[0]?.message, ANNOUNCEMENT_CONTENT_LIMIT_ERROR)
})

test('default announcements schema accepts 3000 emoji code points', () => {
  const parsed = announcementSchema.parse({
    content: '😀'.repeat(ANNOUNCEMENT_CONTENT_LIMIT),
    publishDate: '2026-06-16T12:00:00.000Z',
    type: 'default',
    extra: '',
  })

  assert.equal(Array.from(parsed.content).length, ANNOUNCEMENT_CONTENT_LIMIT)
  assert.equal(getAnnouncementContentLength(parsed.content), ANNOUNCEMENT_CONTENT_LIMIT)
})

test('default announcements schema rejects 3001 emoji code points', () => {
  const result = announcementSchema.safeParse({
    content: '😀'.repeat(ANNOUNCEMENT_CONTENT_LIMIT + 1),
    publishDate: '2026-06-16T12:00:00.000Z',
    type: 'default',
    extra: '',
  })

  assert.equal(result.success, false)
  assert.equal(result.error.issues[0]?.message, ANNOUNCEMENT_CONTENT_LIMIT_ERROR)
})
