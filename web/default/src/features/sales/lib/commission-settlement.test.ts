import assert from 'node:assert/strict'
import test from 'node:test'
import {
  calculateSettlementAmountByPercent,
  getSettlementAmountError,
} from './commission-settlement.ts'

test('calculates settlement amount from shortcut percentage with cents precision', () => {
  assert.equal(calculateSettlementAmountByPercent(123.456, 25), 30.86)
  assert.equal(calculateSettlementAmountByPercent(123.456, 100), 123.46)
})

test('validates settlement amount against pending commission', () => {
  assert.equal(getSettlementAmountError(0, 100), 'invalid')
  assert.equal(getSettlementAmountError(100.01, 100), 'too-large')
  assert.equal(getSettlementAmountError(100, 100), null)
})
