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

import { TimelineStrip } from '../timeline-strip'
import type { QualityBucket } from '../../types'

function makeBuckets(): QualityBucket[] {
  return Array.from({ length: 144 }, (_, index) => ({
    start: 1000 + index * 600_000,
    end: 1000 + (index + 1) * 600_000,
    success: 0,
    failure: 0,
    latest_at: 0,
  }))
}

describe('TimelineStrip', () => {
  test('renders one keyboard-focusable button per bucket', () => {
    const buckets = makeBuckets()
    const { container } = render(
      <TimelineStrip buckets={buckets} selectedBucket={null} onSelect={vi.fn()} />
    )
    const buttons = container.querySelectorAll('button')
    expect(buttons).toHaveLength(144)
  })

  test('failure, mixed, success, and empty buckets expose distinct tones', () => {
    const buckets = makeBuckets()
    buckets[0].success = 2
    buckets[1].failure = 1
    buckets[2].success = 5
    buckets[2].failure = 1
    buckets[3].success = 1
    buckets[3].failure = 1
    const { container } = render(
      <TimelineStrip buckets={buckets} selectedBucket={null} onSelect={vi.fn()} />
    )
    const buttons = container.querySelectorAll('button')
    expect(buttons[0]).toHaveAttribute('data-tone', 'success')
    expect(buttons[1]).toHaveAttribute('data-tone', 'failure')
    expect(buttons[2]).toHaveAttribute('data-tone', 'mostly_success')
    expect(buttons[3]).toHaveAttribute('data-tone', 'mostly_failure')
    expect(buttons[4]).toHaveAttribute('data-tone', 'empty')
  })

  test('a legend labels every tone without relying on color', () => {
    render(
      <TimelineStrip
        buckets={makeBuckets()}
        selectedBucket={null}
        onSelect={vi.fn()}
      />
    )
    for (const label of [
      'Success only',
      'Mostly success',
      'Mostly failure',
      'Failure only',
      'No samples',
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  test('clicking a bucket reports its window, clicking again clears', () => {
    const buckets = makeBuckets()
    const onSelect = vi.fn()
    const { container, rerender } = render(
      <TimelineStrip buckets={buckets} selectedBucket={null} onSelect={onSelect} />
    )
    const target = buckets[10]
    fireEvent.click(container.querySelectorAll('button')[10])
    expect(onSelect).toHaveBeenCalledWith(target)

    rerender(
      <TimelineStrip
        buckets={buckets}
        selectedBucket={target}
        onSelect={onSelect}
      />
    )
    fireEvent.click(container.querySelectorAll('button')[10])
    expect(onSelect).toHaveBeenLastCalledWith(null)
  })

  test('aria-label carries the server success/failure counts', () => {
    const buckets = makeBuckets()
    buckets[4].success = 7
    buckets[4].failure = 3
    const { container } = render(
      <TimelineStrip buckets={buckets} selectedBucket={null} onSelect={vi.fn()} />
    )
    const button = container.querySelectorAll('button')[4]
    expect(button.getAttribute('aria-label')).toContain('7')
    expect(button.getAttribute('aria-label')).toContain('3')
  })
})
