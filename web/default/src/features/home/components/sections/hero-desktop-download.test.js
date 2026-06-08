import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'hero.tsx'), 'utf8')

test('home hero renders a desktop download CTA', () => {
  assert.match(source, /data-testid='home-desktop-download'/)
  assert.match(source, /getDesktopDownloadTarget\(\)/)
  assert.match(source, /data-download-platform=\{desktopDownload\.platform\}/)
  assert.match(source, /href=\{desktopDownload\.href\}/)
})
