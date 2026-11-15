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

import { useQuery } from '@tanstack/react-query'
import { ExternalLink } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { pollOpenDesignLogin } from '../api'
import type { SubscriptionLogin } from '../types'

type SubscriptionLoginPanelProps = {
  readonly challenge: SubscriptionLogin
  readonly onApproved: () => void
  readonly onRestart: () => void
}

export function SubscriptionLoginPanel(props: SubscriptionLoginPanelProps) {
  const { t } = useTranslation()
  const onApproved = props.onApproved
  const login = useQuery({
    queryKey: ['upstream-subscription-login', props.challenge.flow_token],
    queryFn: ({ signal }) =>
      pollOpenDesignLogin(props.challenge.flow_token, signal),
    refetchInterval: (query) =>
      query.state.error ||
      query.state.data?.status === 'approved' ||
      query.state.data?.status === 'expired'
        ? false
        : props.challenge.poll_interval * 1000,
    retry: false,
    meta: { errorToast: false },
  })

  useEffect(() => {
    if (login.data?.status === 'approved') onApproved()
  }, [login.data, onApproved])

  return (
    <div className='bg-muted/40 space-y-3 rounded-lg border p-4'>
      <p className='text-sm font-medium' role='status'>
        {t('Waiting for OpenDesign authorization')}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t('Check that the code on the official page matches:')}{' '}
        <strong className='text-foreground font-mono'>
          {props.challenge.user_code}
        </strong>
      </p>
      <Button
        role='link'
        render={
          <a
            href={props.challenge.activation_url}
            target='_blank'
            rel='noopener noreferrer'
          />
        }
      >
        {t('Open OpenDesign and authorize')}
        <ExternalLink className='size-4' aria-hidden='true' />
      </Button>
      {login.data?.status === 'expired' && (
        <p role='alert' className='text-destructive text-sm'>
          {t('OpenDesign login expired or was cancelled; start again')}
        </p>
      )}
      {login.error && (
        <p role='alert' className='text-destructive text-sm'>
          {getServerErrorMessage(login.error)}
        </p>
      )}
      <div className='flex flex-wrap gap-2'>
        {login.error && (
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => void login.refetch()}
            disabled={login.isFetching}
          >
            {t('Retry')}
          </Button>
        )}
        <Button
          type='button'
          size='sm'
          variant='ghost'
          onClick={props.onRestart}
        >
          {t('Restart login')}
        </Button>
      </div>
    </div>
  )
}
