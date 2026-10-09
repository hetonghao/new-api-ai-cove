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
import { spawnSync, type SpawnSyncOptions } from 'node:child_process'
import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

export const nodeTestFiles = ['src', 'scripts']
  .flatMap((root) =>
    readdirSync(path.join(webRoot, root), { recursive: true }).map((file) =>
      path.join(root, String(file)).replaceAll(path.sep, '/')
    )
  )
  .filter((file) => /\.(test|spec)\.(js|jsx|mjs|ts|tsx)$/.test(file))
  .filter((file) =>
    /from ['"]node:test['"]/.test(
      readFileSync(path.join(webRoot, file), 'utf8')
    )
  )

export function runTests(
  filters: string[] = [],
  run: (
    command: string,
    args: string[],
    options: SpawnSyncOptions
  ) => { status: number | null; error?: Error } = spawnSync
): number {
  let exitCode = 0
  // Run both suites even if one fails, and preserve either failure in the exit code.
  for (const args of [
    [
      'node_modules/tsx/dist/cli.mjs',
      '--tsconfig',
      'tsconfig.app.json',
      '--test',
      ...nodeTestFiles,
    ],
    ['node_modules/vitest/vitest.mjs', 'run', ...filters],
  ]) {
    const result = run(process.execPath, args, {
      cwd: webRoot,
      stdio: 'inherit',
    })
    if (result.error) console.error(result.error)
    if (result.status !== 0) exitCode = 1
  }
  return exitCode
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  process.exitCode = runTests(process.argv.slice(2))
}
