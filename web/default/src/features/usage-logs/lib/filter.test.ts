import assert from 'node:assert/strict'
import test from 'node:test'
import * as filterModule from './filter.ts'
import { buildApiParams } from './utils.ts'

const { buildSearchParams } = filterModule

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

test('buildUsernameFilterSearch keeps current filters and resets page', () => {
  assert.equal(typeof filterModule.buildUsernameFilterSearch, 'function')

  const params = filterModule.buildUsernameFilterSearch(
    {
      page: 3,
      pageSize: 50,
      type: ['4'],
      model: 'gpt-5',
      hideSelf: true,
    },
    ' alice '
  )

  assert.deepEqual(params, {
    page: 1,
    pageSize: 50,
    type: ['4'],
    model: 'gpt-5',
    hideSelf: true,
    username: 'alice',
  })
})
