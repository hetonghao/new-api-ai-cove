import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  LEGAL_CONSENT_MESSAGE,
  getPrimarySubmitButtonState,
  guardPrimarySubmitAction,
} from './legal-consent-guard.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const loginFormSource = readFileSync(join(__dirname, 'LoginForm.jsx'), 'utf8');
const registerFormSource = readFileSync(join(__dirname, 'RegisterForm.jsx'), 'utf8');

test('classic login primary button stays natively clickable during legal block', () => {
  assert.deepEqual(
    getPrimarySubmitButtonState({
      isActuallyDisabled: false,
      requiresLegalConsent: true,
      agreedToLegal: false,
    }),
    {
      disabled: false,
      ariaDisabled: true,
      visuallyDisabled: true,
    },
  );
});

test('classic login primary button click shows legal consent message and intercepts submit during legal block', () => {
  const messages = [];

  const result = guardPrimarySubmitAction({
    requiresLegalConsent: true,
    agreedToLegal: false,
    onBlocked: (message) => {
      messages.push(message);
    },
  });

  assert.deepEqual(messages, [LEGAL_CONSENT_MESSAGE]);
  assert.deepEqual(result, {
    intercepted: true,
    message: LEGAL_CONSENT_MESSAGE,
  });
});

test('classic register submit guard intercepts and prompts instead of continuing submit when unchecked', () => {
  const messages = [];

  const result = guardPrimarySubmitAction({
    requiresLegalConsent: true,
    agreedToLegal: false,
    onBlocked: (message) => {
      messages.push(message);
    },
  });

  assert.deepEqual(messages, [LEGAL_CONSENT_MESSAGE]);
  assert.equal(result.intercepted, true);
});

test('classic register submit guard allows submit when not blocked', () => {
  const messages = [];

  const result = guardPrimarySubmitAction({
    requiresLegalConsent: false,
    agreedToLegal: false,
    onBlocked: (message) => {
      messages.push(message);
    },
  });

  assert.deepEqual(messages, []);
  assert.deepEqual(result, {
    intercepted: false,
    message: null,
  });
});

test('classic primary button keeps real disabled state higher priority than legal visual disable', () => {
  assert.deepEqual(
    getPrimarySubmitButtonState({
      isActuallyDisabled: true,
      requiresLegalConsent: true,
      agreedToLegal: false,
    }),
    {
      disabled: true,
      ariaDisabled: true,
      visuallyDisabled: true,
    },
  );
});

test('classic auth forms wire the primary submit contract into the components', () => {
  assert.match(loginFormSource, /guardPrimarySubmitAction\(\{/);
  assert.match(loginFormSource, /aria-disabled=\{primarySubmitButtonState\.ariaDisabled\}/);

  assert.match(registerFormSource, /guardPrimarySubmitAction\(\{/);
  assert.match(registerFormSource, /aria-disabled=\{primarySubmitButtonState\.ariaDisabled\}/);
});
