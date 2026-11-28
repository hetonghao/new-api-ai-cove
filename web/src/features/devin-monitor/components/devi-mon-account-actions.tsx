/*
Copyright (C) 2023-2026 QuantumNous

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

import type { Ref } from 'react'
import { useTranslation } from 'react-i18next'

import type { DeviMonResetOffer, DeviMonRouting } from '../types'
import { AccountSettingsButton } from './devi-mon-account-settings'
import { DevinAccountToggle } from './devi-mon-devin-toggle'
import { DevinOperationGroup } from './devi-mon-operation-group'
import { AccountRefreshButton } from './devi-mon-refresh-button'
import { DevinRoutingControls } from './devi-mon-status'

export function DeviMonAccountActions(props: {
  readonly email: string
  readonly disabled: boolean | null
  readonly configured: boolean
  readonly routing?: DeviMonRouting | null
  readonly onOpenSettings: () => void
  readonly refreshButton?: Ref<HTMLButtonElement>
  readonly onResetOffer?: (offer: DeviMonResetOffer) => void
}) {
  const { t } = useTranslation()
  const { email, disabled, configured } = props
  return (
    <div
      className='min-w-0 overflow-x-auto'
      aria-label={`${t('Actions')} · ${email}`}
      role='group'
    >
      <div className='ms-auto flex w-max items-center gap-1 px-1 py-0.5'>
        {disabled === null ? (
          <span className='text-muted-foreground text-xs'>
            {t('No Devin account')}
          </span>
        ) : (
          <DevinOperationGroup
            label={`${t('Devin')} · ${email}`}
            className='flex items-center gap-1'
          >
            <DevinAccountToggle
              email={email}
              disabled={disabled}
              configured={configured}
            />
            <DevinRoutingControls
              email={email}
              routing={props.routing}
              configured={configured}
            />
          </DevinOperationGroup>
        )}
        <AccountRefreshButton
          email={email}
          configured={configured}
          buttonRef={props.refreshButton}
          onResetOffer={props.onResetOffer}
        />
        <AccountSettingsButton email={email} onOpen={props.onOpenSettings} />
      </div>
    </div>
  )
}
