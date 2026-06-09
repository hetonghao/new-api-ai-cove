import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const routeSource = readFileSync(
  join(process.cwd(), 'src/routes/_authenticated/desktop-auth.tsx'),
  'utf8'
)
const styles = readFileSync(join(process.cwd(), 'src/styles/index.css'), 'utf8')

test('desktop auth completion page uses the AI Cove Design product mark', () => {
  assert.match(routeSource, /src='\/desgin-logo\.png'/)
  assert.match(routeSource, /className='desktop-auth-brand-logo'/)
  assert.match(routeSource, /className='desktop-auth-brand-ai'/)
  assert.match(routeSource, /className='desktop-auth-brand-gap'/)
  assert.match(routeSource, /aria-label='AI Cove Design'/)
})

test('desktop auth completion page styles match the product landing surface', () => {
  assert.match(styles, /\.desktop-auth-page\s*\{/)
  assert.match(styles, /\.desktop-auth-card\s*\{/)
  assert.match(styles, /\.desktop-auth-brand-copy strong\s*\{[\s\S]*font-family:\s*var\(--home-serif\)/)
  assert.match(styles, /\.desktop-auth-brand-gap\s*\{[\s\S]*width:\s*0\.22em/)
})
