import test from 'node:test';
import assert from 'node:assert/strict';

import {
  USER_TREND_CHART_TYPES,
  createUserTrendChartSpec,
} from './dashboardUserTrendChart.js';

const t = (key) => key;
const renderQuota = (value, digits) => `quota:${value}:${digits}`;

test('defaults user consumption trend to a stacked bar chart', () => {
  const values = [
    { Time: '10:00', User: 'alice', rawQuota: 40 },
    { Time: '10:00', User: 'bob', rawQuota: 60 },
  ];

  const spec = createUserTrendChartSpec({
    values,
    totalQuota: 100,
    t,
    renderQuota,
  });

  assert.equal(spec.type, 'bar');
  assert.equal(spec.stack, true);
  assert.equal(spec.xField, 'Time');
  assert.equal(spec.yField, 'rawQuota');
  assert.equal(spec.seriesField, 'User');
  assert.deepEqual(spec.data, [{ id: 'userTrendData', values }]);
  assert.equal(spec.title.subtext, '总计：quota:100:2');
  assert.equal(spec.bar.state.hover.lineWidth, 1);
});

test('keeps area chart mode available for the user trend', () => {
  const spec = createUserTrendChartSpec({
    chartType: USER_TREND_CHART_TYPES.AREA,
    values: [],
    totalQuota: 0,
    t,
    renderQuota,
  });

  assert.equal(spec.type, 'area');
  assert.equal(spec.stack, false);
  assert.equal(spec.area.style.fillOpacity, 0.15);
  assert.equal(spec.line.style.lineWidth, 2);
  assert.equal(spec.point.visible, false);
});

test('dimension tooltip sorts users, shows total, and includes share', () => {
  const spec = createUserTrendChartSpec({
    values: [],
    totalQuota: 100,
    t,
    renderQuota,
  });

  const rows = spec.tooltip.dimension.updateContent([
    { key: 'alice', value: 40 },
    { key: 'bob', value: 60 },
  ]);

  assert.deepEqual(
    rows.map((row) => [row.key, row.value]),
    [
      ['总计', 'quota:100:4'],
      ['bob', 'quota:60:4 (60.0%)'],
      ['alice', 'quota:40:4 (40.0%)'],
    ],
  );
});
