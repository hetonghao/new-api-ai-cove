import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'currency.ts'), 'utf8')

test('currency helpers clamp all display modes to USD', () => {
  assert.match(source, /const DISPLAY_TYPE_VALUES = \['USD'\] as const/)
  assert.match(source, /return 'USD'/)
  assert.match(source, /displayInCurrency: true/)
  assert.match(source, /quotaDisplayType: 'USD'/)
  assert.match(source, /usdExchangeRate: 1/)
  assert.match(source, /customCurrencySymbol: '\$'/)
  assert.match(source, /return 'USD'/)
})
