import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const source = readFileSync(
  join(process.cwd(), 'src/features/system-settings/general/pricing-section.tsx'),
  'utf8'
)

test('pricing section only exposes USD display mode', () => {
  assert.match(source, /quota_display_type:\s*z\.literal\('USD'\)/)
  assert.match(source, /All quota and billing amounts are displayed in USD \(\$\)\./)
  assert.doesNotMatch(source, /value='CNY'/)
  assert.doesNotMatch(source, /value='TOKENS'/)
  assert.doesNotMatch(source, /value='CUSTOM'/)
})
