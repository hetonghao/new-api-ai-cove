/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import assert from 'node:assert/strict'
import { after, afterEach, test } from 'node:test'

import { Window } from 'happy-dom'
import { FormProvider, useForm } from 'react-hook-form'

import { RISK_PROVIDER_DEFAULT_VALUES } from '../../lib/risk-provider-form'
import type { RiskProviderFormValues } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { RiskProviderFormFields } = await import('../risk-provider-form-fields')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

afterEach(cleanup)

after(() => {
  domWindow.close()
})

function TestForm(props: {
  readonly providerType: 'cloudflare' | 'platform_internal'
}) {
  const form = useForm<RiskProviderFormValues>({
    defaultValues: {
      ...RISK_PROVIDER_DEFAULT_VALUES,
      provider_type: props.providerType,
    },
  })

  return (
    <I18nextProvider i18n={i18n}>
      <FormProvider {...form}>
        <RiskProviderFormFields
          hasCredential={false}
          channels={[]}
          channelsLoading={false}
          channelsError={false}
        />
      </FormProvider>
    </I18nextProvider>
  )
}

function openAdvanced(providerType: 'cloudflare' | 'platform_internal'): void {
  render(<TestForm providerType={providerType} />)
  fireEvent.click(screen.getByRole('button', { name: /Advanced/ }))
}

test('hides Cloudflare quota fields for platform internal models', () => {
  openAdvanced('platform_internal')

  assert.equal(screen.queryByLabelText('Daily Neurons limit'), null)
  assert.equal(screen.queryByLabelText('Daily reset time (UTC+8)'), null)
  assert.ok(screen.getByLabelText('Priority'))
  assert.ok(screen.getByLabelText('Review timeout (ms)'))
})

test('keeps Cloudflare quota fields visible for Cloudflare providers', () => {
  openAdvanced('cloudflare')

  assert.ok(screen.getByLabelText('Daily Neurons limit'))
  assert.ok(screen.getByLabelText('Daily reset time (UTC+8)'))
})
