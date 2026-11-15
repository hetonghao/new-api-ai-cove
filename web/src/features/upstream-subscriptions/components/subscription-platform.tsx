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

import CommandCodeIcon from '@lobehub/icons/es/CommandCode/components/Mono'
import OpenCodeIcon from '@lobehub/icons/es/OpenCode/components/Mono'

import openDesignLogo from '@/assets/brand-icons/opendesign.svg'

import type { UpstreamSubscription } from '../types'

export function SubscriptionPlatform(props: {
  readonly platform: UpstreamSubscription['platform']
  readonly iconOnly?: boolean
}) {
  const labels = {
    opencode: 'OpenCode Go',
    opendesign: 'OpenDesign Go',
    commandcode: 'CommandCode',
  }
  return (
    <span className='inline-flex min-w-0 items-center gap-1.5'>
      <span
        className='flex size-3.5 shrink-0 items-center justify-center'
        aria-hidden='true'
      >
        {props.platform === 'opencode' && <OpenCodeIcon size={14} />}
        {props.platform === 'commandcode' && <CommandCodeIcon size={14} />}
        {props.platform === 'opendesign' && (
          <img
            src={openDesignLogo}
            alt=''
            width={14}
            height={14}
            className='dark:invert'
          />
        )}
      </span>
      {!props.iconOnly && labels[props.platform]}
    </span>
  )
}
