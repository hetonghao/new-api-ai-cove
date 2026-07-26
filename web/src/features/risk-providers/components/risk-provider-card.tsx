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
import { CheckCircle2, Pencil, PlugZap, Power, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { canActivateProvider } from '../lib/risk-provider-form'
import type { RiskProvider } from '../types'

type RiskProviderCardProps = {
  readonly provider: RiskProvider
  readonly pendingAction: 'validate' | 'activate' | 'delete' | null
  readonly onEdit: (provider: RiskProvider) => void
  readonly onValidate: (provider: RiskProvider) => void
  readonly onActivate: (provider: RiskProvider) => void
  readonly onDelete: (provider: RiskProvider) => void
}

export function RiskProviderCard(props: RiskProviderCardProps) {
  const { t } = useTranslation()
  const provider = props.provider

  return (
    <Card className='gap-0 py-0'>
      <CardHeader className='gap-3 border-b p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-1'>
            <CardTitle className='truncate text-base'>
              {provider.name}
            </CardTitle>
            <p className='text-muted-foreground text-xs'>
              Cloudflare Workers AI
            </p>
          </div>
          <div className='flex flex-wrap gap-1.5'>
            {provider.active && <Badge>{t('Active')}</Badge>}
            <Badge variant={provider.validated_at ? 'secondary' : 'outline'}>
              {provider.validated_at ? t('Verified') : t('Not verified')}
            </Badge>
            <Badge
              variant={provider.has_credential ? 'secondary' : 'destructive'}
            >
              {provider.has_credential
                ? t('Credential configured')
                : t('Credential missing')}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4 p-4'>
        <dl className='grid gap-3 text-sm sm:grid-cols-2'>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{t('Model')}</dt>
            <dd className='truncate font-mono text-xs' title={provider.model}>
              {provider.model}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{t('Account ID')}</dt>
            <dd
              className='truncate font-mono text-xs'
              title={provider.account_id}
            >
              {provider.account_id}
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
          {!provider.active && (
            <Button
              size='sm'
              disabled={
                !canActivateProvider(provider) || props.pendingAction !== null
              }
              title={
                provider.validated_at
                  ? undefined
                  : t('Test the connection before activating this provider.')
              }
              onClick={() => props.onActivate(provider)}
            >
              {provider.validated_at ? (
                <Power className='size-4' />
              ) : (
                <CheckCircle2 className='size-4' />
              )}
              {props.pendingAction === 'activate'
                ? t('Activating...')
                : t('Set active')}
            </Button>
          )}
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
