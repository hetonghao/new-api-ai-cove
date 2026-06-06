export const SETTLEMENT_PERCENTAGES = [25, 50, 75, 100] as const

export type SettlementPercentage = (typeof SETTLEMENT_PERCENTAGES)[number]

export type SettlementAmountError = 'invalid' | 'too-large'
export type SettlementDialogMode = 'detail' | 'settle'

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

export function canCreateCommissionSettlement(
  pendingAmount: number | null | undefined,
  commissionRatio: number | null | undefined
): boolean {
  return roundMoney(pendingAmount ?? 0) > 0 && (commissionRatio ?? 0) > 0
}

export function shouldShowSettlementForm(
  mode: SettlementDialogMode,
  canCreateSettlement: boolean
): boolean {
  return mode === 'settle' && canCreateSettlement
}
