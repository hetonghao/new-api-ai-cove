import assert from 'node:assert/strict'
import test from 'node:test'

import { getInitialHomePageContentState } from './home-page-content-state.ts'

test('loads the default homepage immediately when no custom content is cached', () => {
  const state = getInitialHomePageContentState(null)

  assert.deepEqual(state, { content: '', isLoaded: true })
})

test('uses cached custom homepage content before the background refresh finishes', () => {
  const state = getInitialHomePageContentState('# Custom home')

  assert.deepEqual(state, { content: '# Custom home', isLoaded: true })
})
