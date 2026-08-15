/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'

const categoryLabels = [
  'Harassment',
  'Threatening harassment',
  'Hate',
  'Threatening hate',
  'Illicit activity',
  'Violent illicit activity',
  'Self-harm',
  'Self-harm intent',
  'Self-harm instructions',
  'Sexual content',
  'Sexual content involving minors',
  'Violence',
  'Graphic violence',
]

function readLocale(name) {
  const filePath = path.resolve(import.meta.dirname, `${name}.json`)
  return JSON.parse(fs.readFileSync(filePath, 'utf8')).translation
}

test('OpenAI moderation categories have labels in every shipped locale', () => {
  for (const locale of ['en', 'fr', 'ja', 'ru', 'vi', 'zh-TW', 'zh']) {
    const messages = readLocale(locale)
    for (const label of categoryLabels) {
      assert.equal(
        typeof messages[label],
        'string',
        `${locale} is missing "${label}"`
      )
      assert.notEqual(
        messages[label].trim(),
        '',
        `${locale} has an empty label`
      )
    }
  }

  const zh = readLocale('zh')
  assert.equal(zh['Harassment'], '骚扰')
  assert.equal(zh['Threatening harassment'], '威胁性骚扰')
  assert.equal(zh['Sexual content involving minors'], '涉及未成年人的性内容')
  assert.equal(zh['Graphic violence'], '血腥暴力')
})
