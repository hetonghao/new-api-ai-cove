import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'user-auth-form.tsx'), 'utf8')

test('password sign-in renders before alternative sign-in methods', () => {
  const passwordSectionIndex = source.indexOf('{passwordLoginEnabled && (')
  const alternativeSectionIndex = source.indexOf(
    '{hasAlternativeLogin && alternativeLoginMethods}'
  )

  assert.notEqual(passwordSectionIndex, -1)
  assert.notEqual(alternativeSectionIndex, -1)
  assert.ok(
    passwordSectionIndex < alternativeSectionIndex,
    'Expected password sign-in fields to render before the alternative login section.'
  )
})
