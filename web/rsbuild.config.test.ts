import assert from 'node:assert/strict'
import { test } from 'node:test'

import rsbuildConfig from './rsbuild.config'

test('browser config receives the sidecar URL from the launch environment', () => {
  // Given
  const previousSidecarUrl = process.env.VITE_AI_COVE_SIDECAR_BASE_URL
  const sidecarUrl = 'http://127.0.0.1:48788'
  process.env.VITE_AI_COVE_SIDECAR_BASE_URL = sidecarUrl

  try {
    // When
    const config = rsbuildConfig({
      command: 'dev',
      env: '',
      envMode: 'development',
    })

    // Then
    assert.equal(
      config.source?.define?.[
        'import.meta.env.VITE_AI_COVE_SIDECAR_BASE_URL'
      ],
      JSON.stringify(sidecarUrl)
    )
  } finally {
    if (previousSidecarUrl === undefined) {
      delete process.env.VITE_AI_COVE_SIDECAR_BASE_URL
    } else {
      process.env.VITE_AI_COVE_SIDECAR_BASE_URL = previousSidecarUrl
    }
  }
})
