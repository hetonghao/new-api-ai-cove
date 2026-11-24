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

import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { DeviMonRouting } from '../types'

export function DevinAccountStatus({
  disabled,
  routing,
}: {
  readonly disabled: boolean | null
  readonly routing?: DeviMonRouting | null
}) {
  const { t, i18n } = useTranslation()
  if (disabled === null) {
    return (
      <StatusBadge
        size='sm'
        copyable={false}
        variant='neutral'
        label={t('No Devin account')}
      />
    )
  }
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <>
      <StatusBadge
        size='sm'
        copyable={false}
        variant={disabled ? 'danger' : 'success'}
        label={disabled ? t('Devin disabled') : t('Devin enabled')}
      />
      {routing ? (
        <span className='text-muted-foreground inline-flex flex-wrap items-baseline gap-x-2 text-xs tabular-nums'>
          <span className='whitespace-nowrap'>
            {t('Priority')}{' '}
            <span className='text-foreground'>
              {routing.priority === null
                ? `${t('Default')} 0`
                : formatNumber(routing.priority, locale)}
            </span>
          </span>
          <span className='whitespace-nowrap'>
            {t('Weight')}{' '}
            <span className='text-foreground'>
              {routing.weight === null
                ? `${t('Default')} 1`
                : formatNumber(routing.weight, locale)}
            </span>
          </span>
        </span>
      ) : (
        <span className='text-muted-foreground text-xs'>
          {t('Priority')} / {t('Weight')}: {t('Not fetched yet')}
        </span>
      )}
    </>
  )
}
