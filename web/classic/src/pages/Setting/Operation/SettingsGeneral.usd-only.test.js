import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const source = readFileSync(
  join(process.cwd(), 'src/pages/Setting/Operation/SettingsGeneral.jsx'),
  'utf8'
)

test('classic general settings keep quota display locked to USD', () => {
  assert.match(source, /currentInputs\['general_setting\.quota_display_type'\] = 'USD'/)
  assert.match(source, /currentInputs\.DisplayInCurrencyEnabled = true/)
  assert.match(source, /USD \(\$\)/)
  assert.doesNotMatch(source, /value='CNY'/)
  assert.doesNotMatch(source, /value='TOKENS'/)
  assert.doesNotMatch(source, /value='CUSTOM'/)
})
