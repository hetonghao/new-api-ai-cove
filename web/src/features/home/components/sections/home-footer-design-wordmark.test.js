/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

const source = readFileSync(
  join(process.cwd(), 'src/features/home/components/sections/home-footer.tsx'),
  'utf8'
)
const strandsSource = readFileSync(
  join(
    process.cwd(),
    'src/features/home/components/sections/home-footer-strands.tsx'
  ),
  'utf8'
)
const styles = readFileSync(join(process.cwd(), 'src/styles/index.css'), 'utf8')

test('home footer uses the shared AI Cove Design wordmark', () => {
  assert.match(
    source,
    /className='ai-cove-design-wordmark home-footer-design-wordmark'/
  )
  assert.match(source, /aria-label='AI {2}Cove Design'/)
  assert.match(source, /className='ai-cove-design-wordmark__prefix'/)
  assert.match(
    source,
    /className='ai-cove-design-wordmark__space ai-cove-design-wordmark__space--after-prefix'/
  )
  assert.match(source, /className='ai-cove-design-wordmark__image'/)
  assert.match(source, /className='ai-cove-design-wordmark__canvas'/)
  assert.doesNotMatch(source, /<strong>\{t\('AI Cove Design'\)\}<\/strong>/)
})

test('home footer product wordmark keeps the product landing style', () => {
  assert.match(
    styles,
    /\.home-footer-design-wordmark,\s*\n\.home-footer-product-name/
  )
  assert.match(
    styles,
    /\.ai-cove-design-wordmark__prefix\s*\{[\s\S]*font-style:\s*italic/
  )
  assert.match(
    styles,
    /\.ai-cove-design-wordmark__image\s*\{[\s\S]*background-clip:\s*text/
  )
  assert.match(
    styles,
    /\.home-footer-product-copy p\s*\{[\s\S]*text-wrap:\s*balance/
  )
})

test('home footer connects AI Cove to Design and Turbo with Strands', () => {
  assert.match(source, /<HomeFooterStrands \/>/)
  assert.match(source, /<ProductCard product='design' \/>/)
  assert.match(source, /<ProductCard product='turbo' \/>/)
  assert.doesNotMatch(source, /home-footer-system-pulse/)
  assert.match(styles, /\.home-footer-connection\s*\{/)
  assert.match(styles, /\.home-footer-product-stack\s*\{/)
})

test('home footer Strands keeps Turbo lifecycle and accessibility guards', () => {
  assert.match(strandsSource, /1320d40a8318ac7d4fe6690c7206ceda8cdd59bd/)
  assert.match(strandsSource, /getContext\('webgl2'/)
  assert.match(strandsSource, /new ResizeObserver\(resize\)/)
  assert.match(strandsSource, /prefers-reduced-motion: reduce/)
  assert.match(strandsSource, /document\.hidden/)
  assert.match(strandsSource, /Math\.min\(window\.devicePixelRatio \|\| 1, 2\)/)
  assert.match(strandsSource, /gl\.deleteBuffer\(buffer\)/)
  assert.match(strandsSource, /gl\.deleteProgram\(program\)/)
  assert.doesNotMatch(strandsSource, /WEBGL_lose_context/)
})

test('shared AI prefix stays readable in dark mode', () => {
  assert.match(
    styles,
    /\.dark \.ai-cove-design-wordmark__prefix\s*\{[\s\S]*color:\s*var\(--home-ink\)/
  )
})
