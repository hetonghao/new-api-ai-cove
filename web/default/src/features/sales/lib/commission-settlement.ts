export const SETTLEMENT_PERCENTAGES = [25, 50, 75, 100] as const

export type SettlementPercentage = (typeof SETTLEMENT_PERCENTAGES)[number]

export type SettlementAmountError = 'invalid' | 'too-large'

export function roundMoney(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.round((value + Number.EPSILON) * 100) / 100
}

export function calculateSettlementAmountByPercent(
  pendingAmount: number,
  percent: number
): number {
  return roundMoney(((Number(pendingAmount) || 0) * percent) / 100)
}

export function getSettlementAmountError(
  amount: number,
  pendingAmount: number
): SettlementAmountError | null {
  if (!Number.isFinite(amount) || amount <= 0) return 'invalid'
  if (roundMoney(amount) > roundMoney(pendingAmount)) return 'too-large'
  return null
}
