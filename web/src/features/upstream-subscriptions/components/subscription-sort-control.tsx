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

import { ArrowDown, ArrowUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export function SubscriptionSortControl<T extends string>(props: {
  readonly options: { value: T; label: string }[]
  readonly value: T
  readonly descending: boolean
  readonly onChange: (value: T) => void
  readonly onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <ButtonGroup aria-label={t('Sort subscriptions by')} className='shrink-0'>
      <Select
        value={props.value}
        onValueChange={(value) => {
          if (value) props.onChange(value)
        }}
        items={props.options}
      >
        <SelectTrigger
          aria-label={t('Sort subscriptions by')}
          className='bg-background min-w-24'
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {props.options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant='outline'
        size='icon'
        aria-label={props.descending ? t('Descending') : t('Ascending')}
        title={props.descending ? t('Descending') : t('Ascending')}
        onClick={props.onToggle}
      >
        {props.descending ? (
          <ArrowDown aria-hidden='true' />
        ) : (
          <ArrowUp aria-hidden='true' />
        )}
      </Button>
    </ButtonGroup>
  )
}
