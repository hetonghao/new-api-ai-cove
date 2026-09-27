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
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export function ImagineSourceMarker() {
  const { t } = useTranslation()
  const label = t('From AI Cove Imagine skill')

  return (
    <TooltipProvider delay={150}>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type='button'
              className='text-chart-4 hover:bg-chart-4/15 focus-visible:ring-chart-4/50 inline-flex size-4 cursor-default items-center justify-center rounded-full transition-transform duration-200 hover:scale-110 focus-visible:ring-2 focus-visible:outline-none'
              aria-label={label}
            >
              <Sparkles className='fill-chart-4/20 size-3' aria-hidden='true' />
            </button>
          }
        />
        <TooltipContent>
          <p className='text-xs'>{label}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
