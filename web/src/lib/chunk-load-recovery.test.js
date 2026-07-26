import assert from 'node:assert/strict'
import test from 'node:test'

import {
  clearChunkLoadRecoveryMarker,
  getChunkReloadKey,
  isChunkLoadError,
  recoverFromChunkLoadError,
} from './chunk-load-recovery.ts'

function createRuntime(options = {}) {
  const values = new Map()
  const reloads = []
  const runtime = {
    buildRevision: options.buildRevision ?? 'rv.test-build',
    location: {
      pathname: options.pathname ?? '/dashboard',
      reload() {
        reloads.push('reload')
      },
    },
    now() {
      return 1782129673000
    },
    storage: {
      getItem(key) {
        return values.has(key) ? values.get(key) : null
      },
      removeItem(key) {
        values.delete(key)
      },
      setItem(key, value) {
        values.set(key, String(value))
      },
    },
  }

  return { reloads, runtime, values }
}

function createChunkError() {
  const error = new Error(
    'Loading chunk 9707 failed. (missing: https://ai-cove.com/static/js/async/9707.b07381301c.js)'
  )
  error.name = 'ChunkLoadError'
  return error
}

test('detects only missing async chunk load failures', () => {
  assert.equal(isChunkLoadError(createChunkError()), true)
  assert.equal(isChunkLoadError(new Error('Internal Server Error')), false)
  assert.equal(
    isChunkLoadError({
      name: 'ChunkLoadError',
      message: 'Loading chunk 9707 failed.',
    }),
    false
  )
})

test('reloads once and stores a path scoped marker for the first chunk failure', () => {
  const { reloads, runtime, values } = createRuntime()
  const key = getChunkReloadKey(runtime)

  assert.equal(recoverFromChunkLoadError(createChunkError(), runtime), true)

  assert.deepEqual(reloads, ['reload'])
  assert.equal(values.get(key), '1782129673000')
  assert.equal(key, 'chunk-reload:rv.test-build:/dashboard')
})

test('does not reload again when the same build and path already has a marker', () => {
  const { reloads, runtime, values } = createRuntime()
  values.set(getChunkReloadKey(runtime), '1782129673000')

  assert.equal(recoverFromChunkLoadError(createChunkError(), runtime), false)

  assert.deepEqual(reloads, [])
})

test('does not reload for non chunk errors', () => {
  const { reloads, runtime, values } = createRuntime()

  assert.equal(recoverFromChunkLoadError(new Error('boom'), runtime), false)

  assert.deepEqual(reloads, [])
  assert.equal(values.size, 0)
})

test('clears the current path marker after a successful app mount', () => {
  const { runtime, values } = createRuntime()
  const key = getChunkReloadKey(runtime)
  values.set(key, '1782129673000')

  clearChunkLoadRecoveryMarker(runtime)

  assert.equal(values.has(key), false)
})
