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
import { Pencil, PlugZap, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { formatNumber } from '@/lib/format'

import type { RiskProvider } from '../types'

type RiskProviderCardProps = {
  readonly provider: RiskProvider
  readonly pendingAction: 'validate' | 'delete' | 'activate' | null
  readonly onEdit: (provider: RiskProvider) => void
  readonly onValidate: (provider: RiskProvider) => void
  readonly onDelete: (provider: RiskProvider) => void
  readonly onToggleActive: (provider: RiskProvider, active: boolean) => void
}

function getCurrentStatusLabel(
  provider: RiskProvider,
  t: (key: string) => string
) {
  if (provider.current_status === 'circuit_open') return t('Circuit open')
  if (provider.current_status === 'daily_exhausted') {
    return t('Daily quota exhausted')
  }
  return t('Normal')
}

function getCurrentStatusVariant(provider: RiskProvider) {
  if (provider.current_status === 'circuit_open') return 'warning' as const
  if (provider.current_status === 'daily_exhausted') {
    return 'destructive' as const
  }
  return 'secondary' as const
}

export function RiskProviderCard(props: RiskProviderCardProps) {
  const { t } = useTranslation()
  const provider = props.provider
  let providerTypeLabel = 'Cloudflare Workers AI'
  if (provider.provider_type === 'openai') {
    providerTypeLabel = 'OpenAI Moderation'
  } else if (provider.provider_type === 'platform_internal') {
    providerTypeLabel = t('Platform internal model')
  }

  return (
    <Card className='border-border/60 min-w-0 gap-0 border py-0 ring-0'>
      <CardHeader className='gap-3 border-b p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-1'>
            <CardTitle className='truncate text-base'>
              {provider.name}
            </CardTitle>
            <p className='text-muted-foreground text-xs'>{providerTypeLabel}</p>
          </div>
          <div className='flex flex-wrap gap-1.5'>
            <Badge variant={provider.validated_at ? 'secondary' : 'outline'}>
              {provider.validated_at ? t('Verified') : t('Not verified')}
            </Badge>
            {provider.provider_type === 'platform_internal' ? (
              <Badge
                variant={provider.system_managed ? 'secondary' : 'destructive'}
              >
                {provider.system_managed
                  ? t('System token managed')
                  : t('System token unavailable')}
              </Badge>
            ) : (
              <Badge
                variant={provider.has_credential ? 'secondary' : 'destructive'}
              >
                {provider.has_credential
                  ? t('Credential configured')
                  : t('Credential missing')}
              </Badge>
            )}
            <Badge variant={getCurrentStatusVariant(provider)}>
              {getCurrentStatusLabel(provider, t)}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4 p-4'>
        <dl className='grid gap-3 text-sm sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Priority')}</dt>
            <dd className='tabular-nums'>{provider.priority}</dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{t('Model')}</dt>
            <dd className='truncate font-mono text-xs' title={provider.model}>
              {provider.model}
            </dd>
          </div>
          {provider.provider_type === 'platform_internal' && (
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>
                {t('Platform channel')}
              </dt>
              <dd className='truncate font-mono text-xs'>
                #{provider.channel_id}
              </dd>
            </div>
          )}
          {provider.provider_type === 'openai' && (
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Base URL')}</dt>
              <dd
                className='truncate font-mono text-xs'
                title={provider.base_url}
              >
                {provider.base_url}
              </dd>
            </div>
          )}
          {provider.provider_type === 'cloudflare' && (
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>
                {t('Account ID')}
              </dt>
              <dd
                className='truncate font-mono text-xs'
                title={provider.account_id}
              >
                {provider.account_id}
              </dd>
            </div>
          )}
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Daily Neurons')}
            </dt>
            <dd className='tabular-nums'>
              {provider.provider_type === 'cloudflare'
                ? `${formatNumber(provider.daily_neurons_used)} / ${formatNumber(provider.daily_neurons_limit)}`
                : t('Not applicable')}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Daily reset time')}
            </dt>
            <dd className='tabular-nums'>
              {provider.provider_type === 'cloudflare'
                ? `${provider.daily_reset_time} UTC+8`
                : t('Not applicable')}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Review timeout')}
            </dt>
            <dd className='tabular-nums'>{provider.timeout_ms} ms</dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Circuit breaker')}
            </dt>
            <dd className='tabular-nums'>
              {t('{{count}} failures / {{seconds}} seconds', {
                count: provider.failure_threshold,
                seconds: provider.cooldown_seconds,
              })}
            </dd>
          </div>
        </dl>
        <div className='bg-muted/30 flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
          <div>
            <div className='text-sm font-medium'>{t('Enabled')}</div>
            <div className='text-muted-foreground text-xs'>
              {provider.active
                ? t('Provider can receive reviews.')
                : t('Provider is manually disabled.')}
            </div>
          </div>
          <Switch
            checked={provider.active}
            disabled={
              props.pendingAction !== null || provider.validated_at === null
            }
            onCheckedChange={(active) => props.onToggleActive(provider, active)}
            aria-label={
              provider.active ? t('Disable provider') : t('Enable provider')
            }
          />
        </div>
        <div className='flex flex-wrap gap-2 border-t pt-4'>
          <Button
            size='sm'
            variant='outline'
            onClick={() => props.onEdit(provider)}
          >
            <Pencil className='size-4' />
            {t('Edit')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={props.pendingAction !== null}
            onClick={() => props.onValidate(provider)}
          >
            <PlugZap className='size-4' />
            {props.pendingAction === 'validate'
              ? t('Testing...')
              : t('Test connection')}
          </Button>
          <Button
            size='sm'
            variant='destructive'
            disabled={props.pendingAction !== null}
            onClick={() => props.onDelete(provider)}
          >
            <Trash2 className='size-4' />
            {t('Delete')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
