import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'

function readLocale(name) {
  const filePath = path.resolve(import.meta.dirname, `${name}.json`)
  const locale = JSON.parse(fs.readFileSync(filePath, 'utf8'))
  return locale.translation
}

test('general error contact guidance exists in all shipped locales', () => {
  const locales = ['en', 'fr', 'ja', 'ru', 'vi', 'zh']
  const key =
    'If this keeps happening, please contact customer support or your administrator.'

  for (const locale of locales) {
    const messages = readLocale(locale)
    assert.equal(
      typeof messages[key],
      'string',
      `${locale} is missing "${key}"`
    )
    assert.notEqual(
      messages[key].trim(),
      '',
      `${locale} has an empty "${key}" translation`
    )
  }

  assert.equal(
    readLocale('zh')[key],
    '如果问题持续出现，请联系客服或管理员。'
  )
})
