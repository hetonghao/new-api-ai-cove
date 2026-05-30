/*
Copyright (C) 2025 QuantumNous

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
};

export const DEFAULT_USER_TREND_CHART_TYPE = USER_TREND_CHART_TYPES.BAR;

const normalizeUserTrendChartType = (chartType) =>
  chartType === USER_TREND_CHART_TYPES.AREA
    ? USER_TREND_CHART_TYPES.AREA
    : DEFAULT_USER_TREND_CHART_TYPE;

const formatShare = (value, total) => {
  if (!total) return '0.0%';
  return `${((value / total) * 100).toFixed(1)}%`;
};

const createUserTrendTooltip = (t, renderQuota) => ({
  mark: {
    content: [
      {
        key: (datum) => datum['User'],
        value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
      },
    ],
  },
  dimension: {
    content: [
      {
        key: (datum) => datum['User'],
        value: (datum) => datum['rawQuota'] || 0,
      },
    ],
    updateContent: (array) => {
      array.sort((a, b) => b.value - a.value);
      let sum = 0;
      const parsedValues = array.map((item) => {
        let value = parseFloat(item.value);
        if (isNaN(value)) value = 0;
        sum += value;
        return value;
      });

      for (let i = 0; i < array.length; i++) {
        array[i].value = `${renderQuota(parsedValues[i], 4)} (${formatShare(
          parsedValues[i],
          sum,
        )})`;
      }
      array.unshift({
        key: t('总计'),
        value: renderQuota(sum, 4),
      });
      return array;
    },
  },
});

export const createUserTrendChartSpec = ({
  chartType = DEFAULT_USER_TREND_CHART_TYPE,
  values = [],
  totalQuota = 0,
  t,
  renderQuota,
  colors = [],
}) => {
  const normalizedChartType = normalizeUserTrendChartType(chartType);
  const isBarChart = normalizedChartType === USER_TREND_CHART_TYPES.BAR;

  const spec = {
    type: normalizedChartType,
    data: [{ id: 'userTrendData', values }],
    xField: 'Time',
    yField: 'rawQuota',
    seriesField: 'User',
    stack: isBarChart,
    legends: { visible: true, selectMode: 'single' },
    title: {
      visible: true,
      text: t('用户消耗趋势'),
      subtext: `${t('总计')}：${renderQuota(totalQuota, 2)}`,
    },
    axes: [
      {
        orient: 'left',
        label: {
          formatMethod: (value) => renderQuota(value, 2),
        },
      },
    ],
    tooltip: createUserTrendTooltip(t, renderQuota),
    color: { type: 'ordinal', range: colors },
  };

  if (isBarChart) {
    return {
      ...spec,
      bar: {
        state: {
          hover: {
            stroke: '#000',
            lineWidth: 1,
          },
        },
      },
    };
  }

  return {
    ...spec,
    area: { style: { fillOpacity: 0.15 } },
    line: { style: { lineWidth: 2 } },
    point: { visible: false },
  };
};
