import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'hero.tsx'), 'utf8')
const styles = readFileSync(
  path.resolve(__dirname, '../../../../styles/index.css'),
  'utf8'
)

test('home hero renders Design and Turbo as sibling extension app CTAs', () => {
  const turboAnchor = source.match(
    /<a\s+([^>]*data-testid='home-turbo-download'[^>]*)>/
  )?.[1]

  assert.match(source, /className='home-extension-app-actions'/)
  assert.match(source, /Extension apps by AI Cove/)
  assert.match(source, /data-testid='home-desktop-download'/)
  assert.match(source, /data-testid='home-turbo-download'/)
  assert.match(source, /getDesktopDownloadTarget\(\)/)
  assert.match(source, /getTurboDesktopDownloadTarget\(\)/)
  assert.match(source, /data-download-platform=\{desktopDownload\.platform\}/)
  assert.match(source, /data-download-platform=\{turboDownload\.platform\}/)
  assert.match(source, /href=\{desktopDownload\.href\}/)
  assert.match(source, /href=\{turboDownload\.href\}/)
  assert.match(source, /src='\/desgin-logo\.png'/)
  assert.match(source, /src='\/turbo-icon\.png'/)
  assert.match(source, /t\('Create AI images on canvas'\)/)
  assert.match(source, /t\('OpenAI model acceleration engine'\)/)
  assert.match(turboAnchor ?? '', /\bdownload\b/)
  assert.match(turboAnchor ?? '', /href=\{turboDownload\.href\}/)
  assert.doesNotMatch(source, /Preparing release|disabled/)
  assert.match(source, /className='home-desktop-download-icon'/)
})

test('extension app buttons use full-height icon segments', () => {
  const iconSegments = source.match(/className='home-extension-app-icon'/g)
  const buttonRule = styles.match(
    /(?:^|\n)\.home-extension-app-button \{([^}]*)\}/
  )?.[1]
  const iconSegmentRule = styles.match(
    /\.home-extension-app-icon \{([^}]*)\}/
  )?.[1]
  const iconRule = styles.match(/\.home-desktop-download-icon \{([^}]*)\}/)?.[1]

  assert.equal(iconSegments?.length, 2)
  assert.match(buttonRule ?? '', /min-height: 52px;/)
  assert.match(buttonRule ?? '', /overflow: hidden;/)
  assert.match(iconSegmentRule ?? '', /align-self: stretch;/)
  assert.match(iconSegmentRule ?? '', /flex: 0 0 52px;/)
  assert.match(iconRule ?? '', /width: 44px;/)
  assert.match(iconRule ?? '', /height: 44px;/)
})
