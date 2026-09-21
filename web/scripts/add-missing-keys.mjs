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
import fs from 'node:fs/promises'
import path from 'node:path'

const LOCALES_DIR = path.resolve('src/i18n/locales')

function insertSorted(translation, key, value) {
  const entries = Object.entries(translation)
  const idx = entries.findIndex(([existing]) => existing > key)
  if (idx === -1) {
    entries.push([key, value])
  } else {
    entries.splice(idx, 0, [key, value])
  }
  return Object.fromEntries(entries)
}

async function main() {
  const specPath = process.argv[2]
  if (!specPath) {
    throw new Error(
      'usage: node scripts/add-missing-keys.mjs <spec.json> — spec maps each English key to per-locale translations'
    )
  }
  const spec = JSON.parse(await fs.readFile(specPath, 'utf8'))
  const entries = await fs.readdir(LOCALES_DIR, { withFileTypes: true })
  const localeFiles = entries
    .filter((e) => e.isFile() && e.name.endsWith('.json'))
    .map((e) => e.name.replace(/\.json$/i, ''))
    .sort()

  for (const locale of localeFiles) {
    const full = path.join(LOCALES_DIR, `${locale}.json`)
    const json = JSON.parse(await fs.readFile(full, 'utf8'))
    const added = []
    for (const [key, values] of Object.entries(spec)) {
      if (json.translation && Object.hasOwn(json.translation, key)) {
        continue
      }
      const value =
        (typeof values === 'object' && values !== null && values[locale]) ||
        (typeof values === 'object' && values !== null && values.en) ||
        key
      json.translation = insertSorted(json.translation ?? {}, key, value)
      added.push(key)
    }
    await fs.writeFile(full, `${JSON.stringify(json, null, 2)}\n`, 'utf8')
    console.log(`${locale}: +${added.length}`, added)
  }
}

main().catch((err) => {
  console.error(err)
  process.exitCode = 1
})
