import { describe, expect, test } from 'vitest'

import * as filterModule from '../filter.ts'
import { buildApiParams } from '../utils.ts'

const { buildSearchParams } = filterModule

describe('usage log filter parameters', () => {
  test('buildSearchParams keeps hideSelf flag for common usage logs', () => {
    const params = buildSearchParams(
      {
        hideSelf: true,
      },
      'common'
    )

    expect(params.hideSelf).toBe(true)
  })

  test('buildSearchParams maps WS and Turbo source filters', () => {
    const params = buildSearchParams(
      {
        ws: true,
        fromTurbo: true,
      },
      'common'
    )

    expect(params).toEqual({ ws: true, fromTurbo: true })
  })

  test('buildApiParams maps source filters for the logs API', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: {
        ws: true,
        fromTurbo: true,
      },
      columnFilters: [],
      isAdmin: true,
    })

    expect(params.ws).toBe(true)
    expect(params.from_turbo).toBe(true)
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

    expect(params.exclude_user_id).toBe(7)
  })

  test('buildUsernameFilterSearch keeps current filters and resets page', () => {
    expect(filterModule.buildUsernameFilterSearch).toBeTypeOf('function')

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

    expect(params).toEqual({
      page: 1,
      pageSize: 50,
      type: ['4'],
      model: 'gpt-5',
      hideSelf: true,
      username: 'alice',
    })
  })
})
