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

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
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

function ruleBlock(css, selector) {
  const match = new RegExp(`(^|\\n)${escapeRegExp(selector)}\\s*\\{`).exec(
    css
  )
  assert.notEqual(match, null, `missing rule ${selector}`)

  const open = css.indexOf('{', match.index)
  assert.notEqual(open, -1, `missing opening brace for ${selector}`)

  let depth = 0
  for (let index = open; index < css.length; index += 1) {
    if (css[index] === '{') depth += 1
    if (css[index] === '}') depth -= 1
    if (depth === 0) {
      return css.slice(open + 1, index)
    }
  }

  throw new Error(`unterminated rule ${selector}`)
}

test('desktop home extends the floating lines across wide viewports', () => {
  const desktopRule = ruleBlock(read(cssPath), '.home-hero-floating-field')
  const wideDesktopCss = mediaBlock(read(cssPath), '(min-width: 1800px)')

  assert.match(desktopRule, /width:\s*min\(2220px,\s*190vw\)/)
  assert.match(desktopRule, /height:\s*clamp\(420px,\s*46vw,\s*680px\)/)
  assert.match(desktopRule, /rotate\(-5deg\)/)
  assert.match(
    wideDesktopCss,
    /\.home-hero-floating-field\s*\{[^}]*width:\s*min\(calc\(100vw \+ 128px\),\s*2720px\)[^}]*height:\s*min\(980px,\s*max\(760px,\s*38vw\)\)/s
  )
})

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

test('floating lines lower pixel ratio on oversized canvases', () => {
  const source = read(floatingLinesPath)

  assert.match(source, /function getAdaptivePixelRatio/)
  assert.match(source, /const SOFT_CANVAS_AREA = 1_800_000/)
  assert.match(source, /const HUGE_CANVAS_AREA = 2_300_000/)
  assert.match(source, /return Math\.min\(preferredPixelRatio, 1\.25\)/)
  assert.match(source, /return Math\.min\(preferredPixelRatio, 1\.1\)/)
})
