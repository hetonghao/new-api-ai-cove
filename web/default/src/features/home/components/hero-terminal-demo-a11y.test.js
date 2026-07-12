import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(
  path.join(__dirname, 'hero-terminal-demo.tsx'),
  'utf8'
)
const styles = readFileSync(
  path.join(__dirname, '../../../styles/index.css'),
  'utf8'
)

function contrastRatio(foreground, background) {
  const luminance = (value) => {
    const [red, green, blue] = value
      .match(/[\da-f]{2}/gi)
      .map((channel) => Number.parseInt(channel, 16) / 255)
      .map((channel) =>
        channel <= 0.04045
          ? channel / 12.92
          : ((channel + 0.055) / 1.055) ** 2.4
      )
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue
  }
  const lighter = Math.max(luminance(foreground), luminance(background))
  const darker = Math.min(luminance(foreground), luminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

test('hero terminal exposes its demo selectors as an ordinary pressed button group', () => {
  assert.match(source, /useTranslation\(\)/)
  assert.match(source, /role='group'/)
  assert.match(source, /aria-label=\{`\$\{t\('API'\)\} \$\{t\('Example'\)\}`\}/)
  assert.match(source, /aria-pressed=\{isActive\}/)
  assert.doesNotMatch(source, /role='tablist'/)
  assert.doesNotMatch(source, /role='tab'/)
})

test('hero terminal active tones meet AA text contrast', () => {
  const tones = [...source.matchAll(/^\s+tone: '(#[\da-f]{6})',$/gim)].map(
    (match) => match[1]
  )

  assert.equal(tones.length, 4)
  for (const tone of tones) {
    assert.ok(
      contrastRatio(tone, '#faf5ef') >= 4.5,
      `${tone} does not meet 4.5:1 contrast on #faf5ef`
    )
  }
})

test('hero terminal uses its soft tones for dark active tabs', () => {
  const softTones = [
    ...source.matchAll(/^\s+toneSoft: '(#[\da-f]{6})',$/gim),
  ].map((match) => match[1])

  assert.equal(softTones.length, 4)
  for (const tone of softTones) {
    assert.ok(
      contrastRatio(tone, '#332e2a') >= 4.5,
      `${tone} does not meet 4.5:1 contrast on #332e2a`
    )
  }
  assert.match(
    styles,
    /\.dark \.home-api-tab\.active\s*\{[\s\S]*?border-bottom-color:\s*var\(--tone-soft\);[\s\S]*?color:\s*var\(--tone-soft\);[\s\S]*?\}/
  )
})
