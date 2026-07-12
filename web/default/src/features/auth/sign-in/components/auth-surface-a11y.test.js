import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const userAuthFormSource = readFileSync(
  path.join(__dirname, 'user-auth-form.tsx'),
  'utf8'
)
const passwordInputSource = readFileSync(
  path.join(__dirname, '../../../../components/password-input.tsx'),
  'utf8'
)
const styles = readFileSync(
  path.join(__dirname, '../../../../styles/index.css'),
  'utf8'
)

const localeNames = ['en', 'fr', 'ja', 'ru', 'vi', 'zh', 'zh-TW']

function parseColor(value) {
  if (value.startsWith('#')) {
    return value
      .match(/[\da-f]{2}/gi)
      .map((channel) => Number.parseInt(channel, 16) / 255)
      .map((channel) =>
        channel <= 0.04045
          ? channel / 12.92
          : ((channel + 0.055) / 1.055) ** 2.4
      )
  }

  const match = value.match(/oklch\(([\d.]+)%\s+([\d.]+)\s+([\d.]+)\)/)
  assert.ok(match, `Unsupported CSS color: ${value}`)
  const lightness = Number(match[1]) / 100
  const chroma = Number(match[2])
  const hue = (Number(match[3]) * Math.PI) / 180
  const a = chroma * Math.cos(hue)
  const b = chroma * Math.sin(hue)
  const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3

  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ].map((channel) => Math.min(1, Math.max(0, channel)))
}

function contrastRatio(foreground, background) {
  const luminance = (value) => {
    const [red, green, blue] = parseColor(value)
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue
  }
  const lighter = Math.max(luminance(foreground), luminance(background))
  const darker = Math.min(luminance(foreground), luminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

test('password sign-in exposes browser autofill metadata', () => {
  // Given the public sign-in form source
  // When browsers inspect the username and password controls
  // Then both controls identify their credential purpose.
  assert.match(
    userAuthFormSource,
    /name='username'[\s\S]*?<Input[\s\S]*?autoComplete='username'[\s\S]*?\{\.\.\.field\}/
  )
  assert.match(
    userAuthFormSource,
    /name='password'[\s\S]*?<PasswordInput[\s\S]*?autoComplete='current-password'[\s\S]*?\{\.\.\.field\}/
  )
})

test('password visibility control is keyboard reachable and localized', () => {
  // Given the shared password input
  // When keyboard and screen-reader users reach its visibility control
  // Then it stays in tab order and names the next state in the active locale.
  assert.match(passwordInputSource, /useTranslation\(\)/)
  assert.doesNotMatch(passwordInputSource, /tabIndex=\{-1\}/)
  assert.match(
    passwordInputSource,
    /aria-label=\{\s*showPassword\s*\?\s*t\('Hide password'\)\s*:\s*t\('Show password'\)\s*\}/
  )

  for (const localeName of localeNames) {
    const locale = JSON.parse(
      readFileSync(
        path.join(__dirname, `../../../../i18n/locales/${localeName}.json`),
        'utf8'
      )
    )
    assert.equal(typeof locale.translation['Show password'], 'string')
    assert.equal(typeof locale.translation['Hide password'], 'string')
  }
})

test('auth controls are visible on the first frame with one short card entrance', () => {
  // Given motion is allowed
  // When the auth page first paints
  // Then child controls are visible and only the card receives a short entrance.
  const duration = styles.match(/animation:\s*authCardEnter (\d+)ms/)?.[1]
  assert.ok(duration)
  assert.ok(Number(duration) >= 180 && Number(duration) <= 260)
  assert.doesNotMatch(styles, /authFormElementEnter/)
  assert.doesNotMatch(styles, /\.ai-cove-auth-form > :nth-child/)

  const firstFrame = styles.match(
    /@keyframes authCardEnter\s*\{\s*from\s*\{([\s\S]*?)\}/
  )?.[1]
  assert.ok(firstFrame)
  assert.doesNotMatch(firstFrame, /opacity:\s*0/)
})

test('auth card mouse follow is bounded to precise pointers and motion preferences', () => {
  const authLayoutSource = readFileSync(
    path.join(__dirname, '../../auth-layout.tsx'),
    'utf8'
  )

  assert.match(authLayoutSource, /requestAnimationFrame/)
  assert.match(authLayoutSource, /\* 8/)
  assert.match(authLayoutSource, /window\.innerWidth/)
  assert.match(authLayoutSource, /window\.innerHeight/)
  assert.doesNotMatch(authLayoutSource, /getBoundingClientRect/)
  assert.match(authLayoutSource, /event\.pointerType !== 'mouse'/)
  assert.match(
    authLayoutSource,
    /\(hover: hover\) and \(pointer: fine\) and \(prefers-reduced-motion: no-preference\)/
  )
  const shellPointerMoveIndex = authLayoutSource.indexOf(
    'onPointerMove={isHomeVariant ? handleAuthPointerMove : undefined}'
  )
  const stageClassIndex = authLayoutSource.indexOf("'ai-cove-auth-stage'")
  const stageOpeningEnd = authLayoutSource.indexOf('>', stageClassIndex)
  assert.ok(
    shellPointerMoveIndex > 0 && shellPointerMoveIndex < stageClassIndex
  )
  assert.doesNotMatch(
    authLayoutSource.slice(stageClassIndex, stageOpeningEnd),
    /onPointer(?:Move|Leave)/
  )
  assert.match(
    styles,
    /translate:\s*var\(--auth-card-shift-x\) var\(--auth-card-shift-y\)/
  )
  assert.match(
    styles,
    /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.ai-cove-auth-card\s*\{[\s\S]*translate:\s*none !important/
  )
})

test('interactive calls to action keep their floating motion', () => {
  // Given the landing and auth call-to-action styles
  // When the controls are idle
  // Then the original floating motion and interaction feedback remain.
  const headerAuthRule = styles.match(
    /\.ai-cove-landing-header \.public-header-auth-button,[\s\S]*?\.public-header-mobile-auth-button\s*\{([\s\S]*?)\}/
  )?.[1]
  const homeButtonRule = styles.match(/\.home-btn\s*\{([\s\S]*?)\}/)?.[1]
  const authSubmitRule = styles.match(
    /\.ai-cove-auth-form \[data-slot='button'\]\[type='submit'\]\s*\{([\s\S]*?)\}/
  )?.[1]

  assert.ok(headerAuthRule)
  assert.ok(homeButtonRule)
  assert.ok(authSubmitRule)

  assert.match(headerAuthRule, /animation:\s*homeButtonFloat 5\.8s/)
  assert.match(homeButtonRule, /animation:\s*homeButtonFloat 5\.4s/)
  assert.match(authSubmitRule, /animation:\s*homeButtonFloat 5\.6s/)
  assert.match(styles, /@keyframes homeButtonFloat/)

  assert.match(
    headerAuthRule,
    /transition:[\s\S]*transform 260ms var\(--home-ease\)/
  )
  assert.match(
    homeButtonRule,
    /transition:[\s\S]*transform 180ms var\(--home-ease\)/
  )
  assert.match(
    styles,
    /\.ai-cove-auth-shell \[data-slot='button'\]\s*\{[\s\S]*?transition:[\s\S]*?transform 180ms var\(--home-ease\)/
  )
  assert.match(
    styles,
    /\.home-btn:hover,[\s\S]*?transform:\s*translateY\(-1px\)/
  )
  assert.match(
    styles,
    /\.ai-cove-auth-form \[data-slot='button'\]\[type='submit'\]:hover,[\s\S]*?transform:\s*translateY\(-1px\)/
  )
  assert.match(
    styles,
    /\.ai-cove-auth-shell \[data-slot='button'\]:active\s*\{[\s\S]*?transform:\s*translateY\(0\) scale\(0\.99\)/
  )
  assert.match(
    styles,
    /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.home-btn,[\s\S]*animation:\s*none !important/
  )
})

test('homepage and auth controls keep a 44px minimum hit area', () => {
  // Given the shared homepage and auth styles
  // When pointer or touch users target interactive controls
  // Then the hit boxes meet the 44px accessibility minimum.
  assert.match(styles, /\.home-btn\s*\{[^}]*min-height:\s*44px/)
  assert.match(styles, /\.home-api-tab\s*\{[^}]*min-height:\s*44px/)
  assert.match(
    styles,
    /\.ai-cove-auth-shell \[data-slot='input'\],[^{]*\{[^}]*height:\s*44px/
  )
  assert.match(
    styles,
    /\.ai-cove-auth-form \[data-slot='button'\]\[type='button'\]\s*\{[^}]*min-height:\s*44px/
  )
  assert.match(passwordInputSource, /className='[^']*size-11[^']*'/)
})

test('password visibility control keeps the auth focus ring', () => {
  // Given the visibility control is reached from the keyboard
  // When focus-visible styles are applied
  // Then the auth-specific override must keep a visible 3px ring.
  assert.match(
    styles,
    /\[data-slot='button'\]\.password-visibility-toggle\[type='button'\]:focus-visible\s*\{\s*box-shadow:\s*0 0 0 3px\s+color-mix\(in oklab, var\(--home-brand-orange\) 62%, var\(--home-ink\)\);\s*\}/
  )
})

test('shared secondary and placeholder colors meet WCAG AA contrast', () => {
  // Given the light and dark warm-gray tokens
  // When they render on the lightest nearby panel surface
  // Then every secondary-text token reaches the 4.5:1 AA threshold.
  const values = [...styles.matchAll(/--home-muted(?:-2)?:\s*([^;]+);/g)].map(
    (match) => match[1].trim()
  )
  assert.ok(values.length > 0)

  for (const value of values) {
    const background = value.startsWith('#') ? '#f4ece2' : 'oklch(28% 0.026 68)'
    assert.ok(
      contrastRatio(value, background) >= 4.5,
      `${value} does not meet 4.5:1 contrast on ${background}`
    )
  }

  assert.match(
    styles,
    /\.ai-cove-home \.code-muted\s*\{[^}]*color:\s*var\(--home-muted\)/
  )
})
