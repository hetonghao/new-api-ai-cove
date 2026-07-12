import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import {
  detectDesktopDownloadPlatform,
  getDesktopDownloadTarget,
  withDesktopDownloadVersion,
} from './desktop-download.ts'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const localeRoot = path.resolve(__dirname, '../../../i18n/locales')

function readLocale(locale: string) {
  const data = JSON.parse(readFileSync(path.join(localeRoot, locale), 'utf8')) as {
    translation: Record<string, string>
  }

  return data.translation
}

test('detects Windows from userAgentData platform first', () => {
  assert.equal(
    detectDesktopDownloadPlatform({
      userAgentDataPlatform: 'Windows',
      platform: 'MacIntel',
      userAgent: 'Mozilla/5.0 (Macintosh)',
    }),
    'windows'
  )
})

test('detects Windows from navigator platform fallback', () => {
  assert.equal(
    detectDesktopDownloadPlatform({
      platform: 'Win32',
    }),
    'windows'
  )
})

test('detects macOS from navigator platform fallback', () => {
  assert.equal(
    detectDesktopDownloadPlatform({
      platform: 'MacIntel',
    }),
    'macos'
  )
})

test('builds the Windows desktop download target', () => {
  assert.deepEqual(
    getDesktopDownloadTarget({
      userAgentDataPlatform: 'Windows',
    }),
    {
      platform: 'windows',
      href: '/downloads/ai-cove-design-desktop-windows.exe',
      labelKey: 'Download AI Cove Design Windows desktop app',
      ariaLabelKey: 'Download AI Cove Design for Windows',
    }
  )
})

test('builds the macOS desktop download target by default', () => {
  assert.deepEqual(getDesktopDownloadTarget({}), {
    platform: 'macos',
    href: '/downloads/ai-cove-design-desktop-macos.dmg',
    labelKey: 'Download AI Cove Design macOS desktop app',
    ariaLabelKey: 'Download AI Cove Design for macOS',
  })
})

test('adds the desktop release version to download URLs', () => {
  assert.equal(
    withDesktopDownloadVersion(
      '/downloads/ai-cove-design-desktop-macos.dmg',
      '0.2.2'
    ),
    '/downloads/ai-cove-design-desktop-macos.dmg?v=0.2.2'
  )
})

test('maps desktop download labels to the requested Chinese copy', () => {
  const zh = readLocale('zh.json')

  assert.equal(
    zh['Download AI Cove Design macOS desktop app'],
    '下载 AI Cove Design macOS 桌面版'
  )
  assert.equal(
    zh['Download AI Cove Design Windows desktop app'],
    '下载 AI Cove Design Windows 桌面版'
  )
})
