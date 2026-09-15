import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const source = readFileSync(path.join(__dirname, 'public-header.tsx'), 'utf8')

test('public header keeps the mobile menu accessible below the lg breakpoint', () => {
  assert.match(
    source,
    /public-header-desktop hidden items-center gap-0\.5 lg:flex/
  )
  assert.match(source, /flex items-center gap-2 lg:hidden/)
  assert.match(source, /className='size-11'/)
  assert.match(source, /aria-expanded=\{mobileOpen\}/)
  assert.match(source, /aria-controls='public-header-mobile-menu'/)
  assert.match(source, /id='public-header-mobile-menu'/)
  assert.match(source, /aria-hidden=\{!mobileOpen\}/)
  assert.match(source, /inert=\{!mobileOpen\}/)
  assert.match(source, /lg:pointer-events-none lg:hidden/)
})

test('public header closes an open mobile menu when entering desktop layout', () => {
  assert.match(source, /window\.matchMedia\('\(min-width: 1024px\)'\)/)
  assert.match(source, /if \(event\.matches\) \{\s+setMobileOpen\(false\)/)
  assert.match(
    source,
    /desktopQuery\.addEventListener\('change', closeMobileMenuOnDesktop\)/
  )
  assert.match(
    source,
    /desktopQuery\.removeEventListener\('change', closeMobileMenuOnDesktop\)/
  )
})

test('public header mobile auth action has a 44px minimum hit target', () => {
  assert.match(
    source,
    /public-header-mobile-auth-button inline-flex min-h-11 items-center/
  )
})

test('public header keeps language and notifications available from sm to lg without duplicate mounts', () => {
  assert.match(source, /useMediaQuery\('\(min-width: 640px\)'\)/)
  assert.match(source, /useMediaQuery\('\(min-width: 1024px\)'\)/)
  assert.match(
    source,
    /const showCompactUtilities =\s+isAtLeastSmall && !isDesktopNavigation/
  )
  assert.match(
    source,
    /const languageSwitcher = showLanguageSwitcher \? <LanguageSwitcher \/> : null/
  )
  assert.match(source, /const notificationPopover = showNotifications \? \(/)
  assert.equal((source.match(/<LanguageSwitcher \/>/g) ?? []).length, 1)
  assert.equal((source.match(/<NotificationPopover/g) ?? []).length, 1)
  assert.match(source, /\{isDesktopNavigation && languageSwitcher\}/)
  assert.match(source, /\{isDesktopNavigation && notificationPopover\}/)
  assert.match(source, /\{showCompactUtilities && languageSwitcher\}/)
  assert.match(source, /\{showCompactUtilities && notificationPopover\}/)
})

test('public header isolates the open menu and restores trigger focus on Escape', () => {
  assert.match(
    source,
    /const mobileMenuTriggerRef = useRef<HTMLButtonElement>\(null\)/
  )
  assert.match(
    source,
    /document\.querySelectorAll<HTMLElement>\('main, footer'\)/
  )
  assert.match(source, /inert: element\.inert/)
  assert.match(source, /element\.inert = true/)
  assert.match(source, /element\.inert = inert/)
  assert.match(source, /if \(event\.key !== 'Escape'\) return/)
  assert.match(source, /if \(event\.key === 'Tab'\) \{/)
  assert.match(
    source,
    /!event\.shiftKey && document\.activeElement === lastFocusTarget/
  )
  assert.match(
    source,
    /event\.shiftKey && document\.activeElement === firstFocusTarget/
  )
  assert.match(
    source,
    /document\.addEventListener\('keydown', handleOpenMenuKeyDown\)/
  )
  assert.match(
    source,
    /document\.removeEventListener\('keydown', handleOpenMenuKeyDown\)/
  )
  assert.match(
    source,
    /window\.requestAnimationFrame\(\(\) => mobileMenuTriggerRef\.current\?\.focus\(\)\)/
  )
  assert.match(source, /ref=\{mobileMenuTriggerRef\}/)
})

test('public header open-menu focus loop covers visible header and overlay controls', () => {
  assert.match(
    source,
    /const OPEN_MENU_FOCUSABLE_SELECTOR =\s*'a\[href\], button:not\(\[disabled\]\), \[tabindex\]:not\(\[tabindex="-1"\]\)'/
  )
  assert.match(source, /const header = trigger\.closest\('header'\)/)
  assert.match(
    source,
    /header\.querySelectorAll<HTMLElement>\(\s*OPEN_MENU_FOCUSABLE_SELECTOR\s*\)/
  )
  assert.match(
    source,
    /overlay\.querySelectorAll<HTMLElement>\(\s*OPEN_MENU_FOCUSABLE_SELECTOR\s*\)/
  )
  assert.match(
    source,
    /const focusTargets = \[\s*\.\.\.headerFocusTargets,\s*\.\.\.overlayFocusTargets,?\s*\]\.filter/
  )
  assert.match(source, /element\.closest\('\[inert\]'\)/)
  assert.match(source, /const style = window\.getComputedStyle\(element\)/)
  assert.match(source, /const rect = element\.getBoundingClientRect\(\)/)
  assert.match(source, /style\.pointerEvents !== 'none'/)
  assert.match(source, /rect\.width > 0 &&\s*rect\.height > 0/)
  assert.doesNotMatch(
    source,
    /const focusTargets = \[trigger, \.\.\.overlayFocusTargets\]/
  )
})
