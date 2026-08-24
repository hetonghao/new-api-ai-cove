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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { CompactDateTimeRangePicker } from '../compact-date-time-range-picker'
import { LogsFilterField, LogsFilterToggle } from '../logs-filter-toolbar'

describe('usage log filter controls', () => {
  test('source switches default off and report independent changes', () => {
    const onWebSocketChange = vi.fn()
    const onTurboChange = vi.fn()

    render(
      <>
        <LogsFilterToggle
          id='ws-filter'
          label='WS'
          checked={false}
          onCheckedChange={onWebSocketChange}
        />
        <LogsFilterToggle
          id='turbo-filter'
          label='From Turbo'
          checked={false}
          onCheckedChange={onTurboChange}
        />
      </>
    )

    const webSocketSwitch = screen.getByRole('switch', { name: 'WS' })
    const turboSwitch = screen.getByRole('switch', { name: 'From Turbo' })
    expect(webSocketSwitch).toHaveAttribute('aria-checked', 'false')
    expect(turboSwitch).toHaveAttribute('aria-checked', 'false')

    fireEvent.click(webSocketSwitch)
    expect(onWebSocketChange.mock.calls[0]?.[0]).toBe(true)
    expect(onTurboChange).not.toHaveBeenCalled()

    fireEvent.click(turboSwitch)
    expect(onTurboChange.mock.calls[0]?.[0]).toBe(true)
  })

  test('desktop time range keeps intrinsic width while mobile stays fluid', () => {
    const rendered = render(
      <LogsFilterField wide fit>
        <CompactDateTimeRangePicker
          start={new Date('2026-08-25T00:00:00')}
          end={new Date('2026-08-25T01:30:00')}
          onChange={() => undefined}
          className='sm:w-fit sm:max-w-full sm:whitespace-nowrap'
        />
      </LogsFilterField>
    )

    expect(rendered.container.firstElementChild).toHaveClass('sm:w-fit')
    expect(screen.getByRole('button')).toHaveClass(
      'w-full',
      'sm:w-fit',
      'sm:max-w-full',
      'sm:whitespace-nowrap'
    )
  })
})
