import assert from 'node:assert/strict'
import test from 'node:test'
import {
  USER_TREND_CHART_TYPES,
  createUserTrendChartSpec,
} from './user-trend-chart.ts'

const tt = (key: string) => key
const formatVal = (value: number, digits = 2) => `quota:${value}:${digits}`

test('defaults user trend to stacked bar chart', () => {
  const values = [
    { Time: '10:00', User: 'alice', rawQuota: 40, Usage: 4 },
    { Time: '10:00', User: 'bob', rawQuota: 60, Usage: 6 },
  ]

  const spec = createUserTrendChartSpec({
    values,
    totalQuota: 100,
    tt,
    formatVal,
    colors: { alice: '#111', bob: '#222' },
  })

  assert.equal(spec.type, 'bar')
  assert.equal(spec.stack, true)
  assert.equal(spec.xField, 'Time')
  assert.equal(spec.yField, 'rawQuota')
  assert.equal(spec.seriesField, 'User')
  assert.deepEqual(spec.data, [{ id: 'userTrendData', values }])
  assert.equal(spec.title.subtext, 'Total: quota:100:2')
  assert.equal(spec.bar.state.hover.lineWidth, 1)
})

test('keeps area mode available for user trend', () => {
  const spec = createUserTrendChartSpec({
    chartType: USER_TREND_CHART_TYPES.AREA,
    values: [],
    totalQuota: 0,
    tt,
    formatVal,
    colors: {},
  })

  assert.equal(spec.type, 'area')
  assert.equal(spec.stack, false)
  assert.equal(spec.area.style.fillOpacity, 0.15)
  assert.equal(spec.line.style.lineWidth, 2)
  assert.equal(spec.point.visible, false)
})

test('dimension tooltip includes sorted quota and share', () => {
  const spec = createUserTrendChartSpec({
    values: [],
    totalQuota: 100,
    tt,
    formatVal,
    colors: {},
  })

  const rows = spec.tooltip.dimension.updateContent([
    { key: 'alice', value: 40 },
    { key: 'bob', value: 60 },
  ])

  assert.deepEqual(
    rows.map((row: { key: string; value: string | number }) => [
      row.key,
      row.value,
    ]),
    [
      ['Total:', 'quota:100:4'],
      ['bob', 'quota:60:4 (60.0%)'],
      ['alice', 'quota:40:4 (40.0%)'],
    ]
  )
})
