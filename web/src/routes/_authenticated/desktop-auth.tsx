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
import { useEffect, useState } from 'react'
import { z } from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { generateAccessToken } from '@/features/profile/api'
import { getSelf } from '@/lib/api'
import { Main } from '@/components/layout'
import { buildDesktopAuthCallbackUrl } from '@/features/desktop-auth/desktop-auth-redirect'

const searchSchema = z.object({
  nonce: z.string().optional(),
  redirect_uri: z.string().optional(),
})

export const Route = createFileRoute('/_authenticated/desktop-auth')({
  component: DesktopAuthPage,
  validateSearch: searchSchema,
})

function DesktopAuthPage() {
  const { nonce, redirect_uri: redirectUri } = Route.useSearch()
  const { t } = useTranslation()
  const [error, setError] = useState('')

  useEffect(() => {
    async function completeDesktopAuth() {
      try {
        const [selfResponse, tokenResponse] = await Promise.all([
          getSelf(),
          generateAccessToken(),
        ])
        const userId = selfResponse?.data?.id
        const token = tokenResponse?.data
        if (!redirectUri || !nonce || !userId || !token) {
          throw new Error(t('Invalid desktop authentication request'))
        }

        window.location.replace(
          buildDesktopAuthCallbackUrl({
            redirectUri,
            nonce,
            token,
            userId,
          })
        )
      } catch (authError) {
        setError(
          authError instanceof Error
            ? authError.message
            : t('Desktop authentication failed')
        )
      }
    }

    void completeDesktopAuth()
  }, [nonce, redirectUri, t])

  return (
    <Main className='desktop-auth-page'>
      <div className='desktop-auth-card'>
        <div className='desktop-auth-brand'>
          <img
            src='/desgin-logo.png'
            alt=''
            className='desktop-auth-brand-logo'
            aria-hidden='true'
          />
          <div className='desktop-auth-brand-copy'>
            <span>{t('Desktop app')}</span>
            <strong className='ai-cove-design-wordmark' aria-label='AI  Cove Design'>
              <span className='ai-cove-design-wordmark__prefix' aria-hidden='true'>
                AI
              </span>
              <span
                className='ai-cove-design-wordmark__space ai-cove-design-wordmark__space--after-prefix'
                aria-hidden='true'
              />
              <span className='ai-cove-design-wordmark__image' aria-hidden='true'>
                Cove
              </span>
              <span className='ai-cove-design-wordmark__space' aria-hidden='true' />
              <span className='ai-cove-design-wordmark__canvas' aria-hidden='true'>
                Design
              </span>
            </strong>
          </div>
        </div>
        <div className='desktop-auth-status' data-error={Boolean(error)}>
          <span className='desktop-auth-status-dot' aria-hidden='true' />
          <div>
            <h1>{t('Connecting AI Cove Design desktop app')}</h1>
            <p>
              {error || t('Please wait while AI Cove signs in the desktop app.')}
            </p>
          </div>
        </div>
      </div>
    </Main>
  )
}
