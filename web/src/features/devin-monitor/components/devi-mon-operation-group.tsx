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

import { type ComponentProps, type ReactNode, useContext } from 'react'
import { useTranslation } from 'react-i18next'

import { PopoverTrigger } from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { DevinOperationMode } from '../operation-mode'

export function DevinOperationGroup({
  label,
  className,
  children,
}: {
  readonly label: string
  readonly className?: string
  readonly children: ReactNode
}) {
  const enabled = useContext(DevinOperationMode)
  const { t } = useTranslation()
  const hint = t('Please turn on operation mode')
  return (
    <Tooltip disabled={enabled}>
      <TooltipTrigger
        closeOnClick={false}
        render={
          <div
            role='group'
            aria-label={label}
            aria-disabled={!enabled || undefined}
            aria-description={enabled ? undefined : hint}
            tabIndex={enabled ? undefined : 0}
            className={cn(
              'max-w-full rounded-md focus-visible:outline-ring focus-visible:outline-2',
              !enabled && 'cursor-not-allowed'
            )}
          />
        }
      >
        <fieldset
          disabled={!enabled}
          className={cn(
            'min-w-0',
            className,
            !enabled &&
              'pointer-events-none [&_:disabled:not([disabled])]:opacity-100'
          )}
        >
          {children}
        </fieldset>
      </TooltipTrigger>
      <TooltipContent role='tooltip'>{hint}</TooltipContent>
    </Tooltip>
  )
}

// Hover still inspects the data; pinning a disclosure is an explicit action.
export function DevinDetailTrigger({
  className,
  ...props
}: Omit<
  ComponentProps<typeof PopoverTrigger>,
  'className' | 'onClickCapture'
> & {
  readonly className?: string
}) {
  const enabled = useContext(DevinOperationMode)
  const { t } = useTranslation()
  return (
    <PopoverTrigger
      {...props}
      className={cn(className, !enabled && 'cursor-not-allowed')}
      title={enabled ? undefined : t('Please turn on operation mode')}
      onClickCapture={
        enabled
          ? undefined
          : (event) => {
              event.preventDefault()
              event.stopPropagation()
            }
      }
    />
  )
}
