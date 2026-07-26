export const LEGAL_CONSENT_MESSAGE = '请先勾选下方的用户协议～'

export function getLegalConsentBlockState(
  requiresLegalConsent: boolean,
  agreedToLegal: boolean
) {
  if (requiresLegalConsent && !agreedToLegal) {
    return {
      blocked: true,
      message: LEGAL_CONSENT_MESSAGE,
    }
  }

  return {
    blocked: false,
    message: null,
  }
}

type PrimarySubmitButtonStateOptions = {
  isActuallyDisabled: boolean
  requiresLegalConsent: boolean
  agreedToLegal: boolean
}

export function getPrimarySubmitButtonState({
  isActuallyDisabled,
  requiresLegalConsent,
  agreedToLegal,
}: PrimarySubmitButtonStateOptions) {
  const blockState = getLegalConsentBlockState(
    requiresLegalConsent,
    agreedToLegal
  )

  return {
    disabled: isActuallyDisabled,
    ariaDisabled: blockState.blocked,
    visuallyDisabled: blockState.blocked,
  }
}

type GuardPrimarySubmitActionOptions = {
  requiresLegalConsent: boolean
  agreedToLegal: boolean
  onBlocked: (message: string) => void
}

export function guardPrimarySubmitAction({
  requiresLegalConsent,
  agreedToLegal,
  onBlocked,
}: GuardPrimarySubmitActionOptions) {
  const blockState = getLegalConsentBlockState(
    requiresLegalConsent,
    agreedToLegal
  )

  if (blockState.blocked && blockState.message) {
    onBlocked(blockState.message)
    return {
      intercepted: true,
      message: blockState.message,
    }
  }

  return {
    intercepted: false,
    message: null,
  }
}
