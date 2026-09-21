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

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { QualityChannelView } from '../types'

type ChannelSwitcherProps = {
  channels: readonly QualityChannelView[]
  value: number | undefined
  onChange: (channelId: number) => void
}

function channelLabel(id: number, name: string, t: (key: string) => string) {
  if (id === 0) return t('Unattributed channel')
  return name || `#${id}`
}

export function ChannelSwitcher(props: ChannelSwitcherProps) {
  const { t } = useTranslation()
  const count = props.channels.length

  if (count === 0) {
    return (
      <span className='text-muted-foreground text-sm'>
        {t('No channels observed yet')}
      </span>
    )
  }
  if (count === 1) {
    const only = props.channels[0]
    return (
      <span className='text-sm font-medium'>
        {channelLabel(only.id, only.name, t)}
      </span>
    )
  }
  return (
    <Tabs
      value={String(props.value ?? props.channels[0].id)}
      onValueChange={(value) => props.onChange(Number(value))}
    >
      <TabsList
        variant='line'
        className='group-data-horizontal/tabs:h-auto h-auto max-w-full flex-wrap justify-start gap-y-2 pb-1'
      >
        {props.channels.map((channel) => (
          <TabsTrigger key={channel.id} value={String(channel.id)}>
            {channelLabel(channel.id, channel.name, t)}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
