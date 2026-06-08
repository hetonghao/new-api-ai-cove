import assert from 'node:assert/strict'
import test from 'node:test'
import {
  detectDesktopDownloadPlatform,
  getDesktopDownloadTarget,
} from './desktop-download.ts'

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
      labelKey: 'Download Windows desktop app',
      ariaLabelKey: 'Download AI-Cove-Design for Windows',
    }
  )
})

test('builds the macOS desktop download target by default', () => {
  assert.deepEqual(getDesktopDownloadTarget({}), {
    platform: 'macos',
    href: '/downloads/ai-cove-design-desktop-macos.dmg',
    labelKey: 'Download macOS desktop app',
    ariaLabelKey: 'Download AI-Cove-Design for macOS',
  })
})
