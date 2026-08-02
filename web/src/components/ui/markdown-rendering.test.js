import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(__dirname, 'markdown.tsx'), 'utf8')

test('shared markdown component applies explicit heading styles instead of relying on prose defaults', () => {
  assert.match(source, /h1:\s*\(\{[^)]*\}\)\s*=>/)
  assert.match(source, /h2:\s*\(\{[^)]*\}\)\s*=>/)
  assert.match(source, /h3:\s*\(\{[^)]*\}\)\s*=>/)
  assert.match(
    source,
    /cn\(\s*'mt-6 mb-3 text-2xl font-semibold tracking-tight'/
  )
  assert.match(
    source,
    /cn\(\s*'mt-5 mb-3 text-xl font-semibold tracking-tight'/
  )
  assert.match(
    source,
    /cn\(\s*'mt-4 mb-2 text-lg font-semibold tracking-tight'/
  )
})
