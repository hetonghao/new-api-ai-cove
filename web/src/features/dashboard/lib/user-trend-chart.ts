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

export const USER_TREND_CHART_TYPES = {
  BAR: 'bar',
  AREA: 'area',
} as const

export type UserTrendChartType =
  (typeof USER_TREND_CHART_TYPES)[keyof typeof USER_TREND_CHART_TYPES]

export const DEFAULT_USER_TREND_CHART_TYPE: UserTrendChartType =
  USER_TREND_CHART_TYPES.BAR

type TooltipLineItem = {
  key: string
  value: string | number
  datum?: Record<string, unknown>
}

type UserTrendValue = {
  Time: string
  User: string
  rawQuota: number
  Usage: number
}

type CreateUserTrendChartSpecOptions = {
  chartType?: UserTrendChartType
  values: UserTrendValue[]
  totalQuota: number
  tt: (key: string) => string
  formatVal: (value: number, digits?: number) => string
  colors: Record<string, string>
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type VChartSpecLike = Record<string, any>

const normalizeUserTrendChartType = (
  chartType?: UserTrendChartType
): UserTrendChartType =>
  chartType === USER_TREND_CHART_TYPES.AREA
    ? USER_TREND_CHART_TYPES.AREA
    : DEFAULT_USER_TREND_CHART_TYPE

const formatShare = (value: number, total: number) => {
  if (!total) return '0.0%'
  return `${((value / total) * 100).toFixed(1)}%`
}

const createTooltip = (
  tt: (key: string) => string,
  formatVal: (value: number, digits?: number) => string
) => ({
  mark: {
    content: [
      {
        key: (datum: Record<string, unknown>) => datum?.User,
        value: (datum: Record<string, unknown>) =>
          formatVal(Number(datum?.rawQuota) || 0, 4),
      },
    ],
  },
  dimension: {
    content: [
      {
        key: (datum: Record<string, unknown>) => datum?.User,
        value: (datum: Record<string, unknown>) => Number(datum?.rawQuota) || 0,
      },
    ],
    updateContent: (array: TooltipLineItem[]) => {
      array.sort((a, b) => (Number(b.value) || 0) - (Number(a.value) || 0))
      let sum = 0
      const parsedValues = array.map((item) => {
        const value = Number(item.value) || 0
        sum += value
        return value
      })

      for (let i = 0; i < array.length; i++) {
        array[i].value = `${formatVal(parsedValues[i], 4)} (${formatShare(
          parsedValues[i],
          sum
        )})`
      }
      array.unshift({
        key: tt('Total:'),
        value: formatVal(sum, 4),
      })
      return array
    },
  },
})

export function createUserTrendChartSpec({
  chartType,
  values,
  totalQuota,
  tt,
  formatVal,
  colors,
}: CreateUserTrendChartSpecOptions): VChartSpecLike {
  const normalizedChartType = normalizeUserTrendChartType(chartType)
  const isBarChart = normalizedChartType === USER_TREND_CHART_TYPES.BAR

  const spec = {
    type: normalizedChartType,
    data: [{ id: 'userTrendData', values }],
    xField: 'Time',
    yField: 'rawQuota',
    seriesField: 'User',
    stack: isBarChart,
    title: {
      visible: true,
      text: tt('User Consumption Trend'),
      subtext: `${tt('Total:')} ${formatVal(totalQuota, 2)}`,
    },
    legends: { visible: true, selectMode: 'single' },
    axes: [
      { orient: 'bottom', type: 'band' },
      {
        orient: 'left',
        type: 'linear',
        label: {
          formatMethod: (value: number) => formatVal(value, 2),
        },
      },
    ],
    tooltip: createTooltip(tt, formatVal),
    color: { specified: colors },
    background: { fill: 'transparent' },
    animation: true,
  }

  if (isBarChart) {
    return {
      ...spec,
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
      },
    }
  }

  return {
    ...spec,
    area: {
      style: {
        fillOpacity: 0.15,
        curveType: 'monotone',
      },
    },
    line: {
      style: {
        lineWidth: 2,
        curveType: 'monotone',
      },
    },
    point: { visible: false },
  }
}
