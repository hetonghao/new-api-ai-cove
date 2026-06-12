import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const source = readFileSync(
  join(process.cwd(), 'src/features/home/components/sections/home-footer.tsx'),
  'utf8'
)
const styles = readFileSync(join(process.cwd(), 'src/styles/index.css'), 'utf8')

test('home footer uses the shared AI Cove Design wordmark', () => {
  assert.match(source, /className='ai-cove-design-wordmark home-footer-design-wordmark'/)
  assert.match(source, /aria-label='AI  Cove Design'/)
  assert.match(source, /className='ai-cove-design-wordmark__prefix'/)
  assert.match(source, /className='ai-cove-design-wordmark__space ai-cove-design-wordmark__space--after-prefix'/)
  assert.match(source, /className='ai-cove-design-wordmark__image'/)
  assert.match(source, /className='ai-cove-design-wordmark__canvas'/)
  assert.doesNotMatch(source, /<strong>\{t\('AI Cove Design'\)\}<\/strong>/)
})

test('home footer product wordmark keeps the product landing style', () => {
  assert.match(styles, /\.home-footer-design-wordmark\s*\{/)
  assert.match(styles, /\.ai-cove-design-wordmark__prefix\s*\{[\s\S]*font-style:\s*italic/)
  assert.match(styles, /\.ai-cove-design-wordmark__image\s*\{[\s\S]*background-clip:\s*text/)
})
