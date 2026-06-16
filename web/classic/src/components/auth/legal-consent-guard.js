export const LEGAL_CONSENT_MESSAGE = '请先勾选下方的用户协议～';

export function getLegalConsentBlockState(
  requiresLegalConsent,
  agreedToTerms,
) {
  if (requiresLegalConsent && !agreedToTerms) {
    return {
      blocked: true,
      message: LEGAL_CONSENT_MESSAGE,
    };
  }

  return {
    blocked: false,
    message: null,
  };
}

export function getPrimarySubmitButtonState({
  isActuallyDisabled,
  requiresLegalConsent,
  agreedToLegal,
}) {
  const blockState = getLegalConsentBlockState(
    requiresLegalConsent,
    agreedToLegal,
  );

  return {
    disabled: isActuallyDisabled,
    ariaDisabled: blockState.blocked,
    visuallyDisabled: blockState.blocked,
  };
}

export function guardPrimarySubmitAction({
  requiresLegalConsent,
  agreedToLegal,
  onBlocked,
}) {
  const blockState = getLegalConsentBlockState(
    requiresLegalConsent,
    agreedToLegal,
  );

  if (blockState.blocked && blockState.message) {
    onBlocked(blockState.message);
    return {
      intercepted: true,
      message: blockState.message,
    };
  }

  return {
    intercepted: false,
    message: null,
  };
}
