import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'general-error.tsx'), 'utf8')

test('general error page no longer exposes GitHub issue feedback UI', () => {
  assert.equal(
    source.includes('Report an issue'),
    false,
    'Expected the error page to remove the "Report an issue" button.'
  )
  assert.equal(
    source.includes('FEEDBACK_URL'),
    false,
    'Expected the error page to stop linking to GitHub Issues.'
  )
})
