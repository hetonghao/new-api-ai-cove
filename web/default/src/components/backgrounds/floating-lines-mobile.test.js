import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const currentDir = dirname(fileURLToPath(import.meta.url))
const cssPath = resolve(currentDir, '../../styles/index.css')
const floatingLinesPath = resolve(currentDir, './floating-lines.tsx')

function read(path) {
  return readFileSync(path, 'utf8')
}

function mediaBlock(css, query) {
  const start = css.indexOf(`@media ${query}`)
  assert.notEqual(start, -1, `missing media query ${query}`)

  const open = css.indexOf('{', start)
  assert.notEqual(open, -1, `missing opening brace for ${query}`)

  let depth = 0
  for (let index = open; index < css.length; index += 1) {
    if (css[index] === '{') depth += 1
    if (css[index] === '}') depth -= 1
    if (depth === 0) {
      return css.slice(open + 1, index)
    }
  }

  throw new Error(`unterminated media query ${query}`)
}

test('mobile home keeps the hero floating lines visible', () => {
  const mobileCss = mediaBlock(read(cssPath), '(max-width: 680px)')

  assert.doesNotMatch(
    mobileCss,
    /\.home-hero-floating-field\s*\{[^}]*display\s*:\s*none\b/s
  )
  assert.match(
    mobileCss,
    /\.home-hero-floating-field\s*\{[^}]*display\s*:\s*block\b/s
  )
  assert.match(
    mobileCss,
    /\.home-hero-floating-field\s*\{[^}]*pointer-events\s*:\s*none\b/s
  )
})

test('floating lines premultiply transparent pixels for mobile WebViews', () => {
  const source = read(floatingLinesPath)

  assert.match(
    source,
    /gl_FragColor\s*=\s*vec4\(lineColor\s*\*\s*lineAlpha,\s*lineAlpha\)/
  )
  assert.match(source, /premultipliedAlpha\s*:\s*true/)
  assert.match(source, /renderer\.setClearAlpha\(0\)/)
})
