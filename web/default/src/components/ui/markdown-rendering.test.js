import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(__dirname, 'markdown.tsx'), 'utf8')

test('shared markdown component uses the upstream marked renderer with sanitization', () => {
  assert.match(source, /new Marked\(/)
  assert.match(source, /DOMPurify\.sanitize/)
  assert.match(source, /dangerouslySetInnerHTML/)
  assert.doesNotMatch(source, /ReactMarkdown/)
})

test('shared markdown component keeps AI Cove overflow-safe display styles', () => {
  assert.match(source, /'prose prose-sm dark:prose-invert max-w-none'/)
  assert.match(source, /'prose-h1:text-2xl prose-h2:text-xl prose-h3:text-lg'/)
  assert.match(source, /'\[overflow-wrap:anywhere\] break-words'/)
})
