import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const source = readFileSync(
  join(process.cwd(), 'src/routes/_authenticated/ai-cove-design/index.tsx'),
  'utf8'
)

test('waits for the authenticated user id before loading the AI Cove Design iframe', () => {
  assert.match(
    source,
    /if \(!userId\) \{[\s\S]*data-testid='ai-cove-design-loading'[\s\S]*return \(/,
    'AI Cove Design should not mount the embedded iframe until the auth store has a user id'
  )
  assert.match(
    source,
    /createAiCoveDesignSidecarUrl\(userId, initialThemeRef\.current\)/,
    'The iframe URL should still be built with the authenticated user id once it is available'
  )
})

test('hands authenticated host credentials to the cross-origin Design iframe', () => {
  assert.match(
    source,
    /const accessToken = useAuthStore\(\(state\) => state\.auth\.accessToken\)/
  )
  assert.match(
    source,
    /event\.source !== iframeRef\.current\?\.contentWindow[\s\S]*event\.origin !== sidecarOrigin/
  )
  assert.match(
    source,
    /postMessage\([\s\S]*type: HOST_CREDENTIALS_MESSAGE_TYPE[\s\S]*token: accessToken[\s\S]*userId[\s\S]*sidecarOrigin/
  )
})
