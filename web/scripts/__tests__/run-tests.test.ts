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
import { expect, test, vi } from 'vitest'

import { nodeTestFiles, runTests } from '../run-tests'

test('discovers Node tests in JS, TS and TSX without claiming Vitest tests', () => {
  expect(nodeTestFiles).toContain('src/lib/chunk-load-recovery.test.js')
  expect(nodeTestFiles).toContain('src/lib/currency-usd-only.test.ts')
  expect(nodeTestFiles).toContain(
    'src/features/home/components/sections/__tests__/home-footer.test.tsx'
  )
  expect(nodeTestFiles).not.toContain(
    'src/features/usage-logs/components/__tests__/usage-facts.test.tsx'
  )
  expect(new Set(nodeTestFiles).size).toBe(nodeTestFiles.length)
})

test.each([
  [0, 0, 0],
  [1, 0, 1],
  [0, 1, 1],
  [null, 0, 1],
])(
  'runs both runners and reports statuses %s / %s as %s',
  (nodeStatus, vitestStatus, expected) => {
    const run = vi
      .fn()
      .mockReturnValueOnce({ status: nodeStatus })
      .mockReturnValueOnce({ status: vitestStatus })
    expect(runTests([], run)).toBe(expected)
    expect(run).toHaveBeenCalledTimes(2)
    expect(run.mock.calls[0][1]).toEqual([
      'node_modules/tsx/dist/cli.mjs',
      '--tsconfig',
      'tsconfig.app.json',
      '--test',
      ...nodeTestFiles,
    ])
    expect(run.mock.calls[1][1]).toEqual([
      'node_modules/vitest/vitest.mjs',
      'run',
    ])
  }
)
