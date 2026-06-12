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
  assert.match(routeSource, /className='ai-cove-design-wordmark'/)
  assert.match(routeSource, /className='ai-cove-design-wordmark__prefix'/)
  assert.match(routeSource, /className='ai-cove-design-wordmark__space ai-cove-design-wordmark__space--after-prefix'/)
  assert.match(routeSource, /className='ai-cove-design-wordmark__image'/)
  assert.match(routeSource, /className='ai-cove-design-wordmark__canvas'/)
  assert.match(routeSource, /aria-label='AI  Cove Design'/)
})

test('desktop auth completion page styles match the product landing surface', () => {
  assert.match(styles, /\.desktop-auth-page\s*\{/)
  assert.match(styles, /\.desktop-auth-card\s*\{/)
  assert.match(styles, /\.ai-cove-design-wordmark\s*\{[\s\S]*font-family:\s*var\(--home-serif\)/)
  assert.match(styles, /\.ai-cove-design-wordmark__prefix\s*\{[\s\S]*font-style:\s*italic/)
  assert.match(styles, /\.ai-cove-design-wordmark__image\s*\{[\s\S]*background:\s*linear-gradient\(100deg, #7e321a 0%, #c65f32 48%, #0f766e 100%\)/)
  assert.match(styles, /\.ai-cove-design-wordmark__space--after-prefix\s*\{[\s\S]*width:\s*0\.34em/)
})
