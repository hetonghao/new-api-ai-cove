import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createAiCoveDesignSidecarUrl } from './ai-cove-sidecar-url'

type WindowLike = {
  location: {
    origin: string
    hostname: string
  }
}

function withWindow<T>(windowLike: WindowLike, fn: () => T): T {
  const globalScope = globalThis as { window?: WindowLike }
  const previousWindow = globalScope.window

  globalScope.window = windowLike

  try {
    return fn()
  } finally {
    globalScope.window = previousWindow
  }
}

test('createAiCoveDesignSidecarUrl appends the current user id', () => {
  withWindow(
    {
      location: {
        origin: 'http://127.0.0.1:4174',
        hostname: '127.0.0.1',
      },
    },
    () => {
      const url = new URL(createAiCoveDesignSidecarUrl(42))

      assert.equal(url.origin, 'http://127.0.0.1:4174')
      assert.equal(url.searchParams.get('base_url'), 'http://127.0.0.1:38080')
      assert.equal(url.searchParams.get('ui_mode'), 'embedded')
      assert.equal(url.searchParams.get('user_id'), '42')
    },
  )
})
