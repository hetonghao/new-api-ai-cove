import assert from 'node:assert/strict'
import test from 'node:test'
import {
  canCreateCommissionSettlement,
  calculateSettlementAmountByPercent,
  getSettlementAmountError,
  shouldShowSettlementForm,
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

test('detects whether a salesperson can create a new commission settlement', () => {
  assert.equal(canCreateCommissionSettlement(100, 10), true)
  assert.equal(canCreateCommissionSettlement(0, 10), false)
  assert.equal(canCreateCommissionSettlement(100, 0), false)
})

test('shows settlement form only for settle mode with pending commission', () => {
  assert.equal(shouldShowSettlementForm('detail', true), false)
  assert.equal(shouldShowSettlementForm('settle', false), false)
  assert.equal(shouldShowSettlementForm('settle', true), true)
})
