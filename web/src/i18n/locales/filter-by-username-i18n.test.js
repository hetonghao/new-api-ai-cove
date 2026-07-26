import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'

function readLocale(name) {
  const filePath = path.resolve(
    import.meta.dirname,
    `${name}.json`
  )
  const locale = JSON.parse(fs.readFileSync(filePath, 'utf8'))
  return locale.translation
}

test('user info filter button key exists in all shipped locales', () => {
  const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh']

  for (const locale of locales) {
    const messages = readLocale(locale)
    assert.equal(
      typeof messages['Filter by this username'],
      'string',
      `${locale} is missing "Filter by this username"`
    )
    assert.notEqual(
      messages['Filter by this username'].trim(),
      '',
      `${locale} has an empty "Filter by this username" translation`
    )
  }

  assert.equal(readLocale('zh')['Filter by this username'], '按此用户名筛选')
})
