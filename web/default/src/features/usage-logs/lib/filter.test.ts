import assert from 'node:assert/strict'
import test from 'node:test'
import { buildSearchParams } from './filter.ts'
import { buildApiParams } from './utils.ts'

test('buildSearchParams keeps hideSelf flag for common usage logs', () => {
  const params = buildSearchParams(
    {
      hideSelf: true,
    },
    'common'
  )

  assert.equal(params.hideSelf, true)
})

test('buildApiParams maps hideSelf to exclude_user_id for admin requests', () => {
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    searchParams: {
      hideSelf: true,
    },
    columnFilters: [],
    isAdmin: true,
    currentUserId: 7,
  })

  assert.equal(params.exclude_user_id, 7)
})
