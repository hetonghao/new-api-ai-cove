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
import { Link } from '@tanstack/react-router'
import { ArrowRight, Download } from 'lucide-react'
import { Trans, useTranslation } from 'react-i18next'

import {
  getDesktopDownloadTarget,
  getTurboDesktopDownloadTarget,
} from '../../lib/desktop-download'
import { HeroFloatingLines } from '../hero-floating-lines'
import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const [oneApiSitePrefix, oneApiSiteSuffix] = t('One API site').split('API')
  const desktopDownload = getDesktopDownloadTarget()
  const turboDownload = getTurboDesktopDownloadTarget()
  const desktopPlatformLabel =
    desktopDownload.platform === 'windows' ? 'Windows' : 'macOS'

  return (
    <section className='home-shell home-hero' aria-labelledby='home-hero-title'>
      <HeroFloatingLines />
      <div className='home-hero-copy landing-animate-fade-up'>
        <div className='home-eyebrow'>AI Cove</div>
        <h1 id='home-hero-title' className='home-hero-title'>
          <span className='home-title-line'>
            {oneApiSitePrefix}
            <span className='home-title-api'>{t('API')}</span>
            {oneApiSiteSuffix}
          </span>
          <span className='home-title-line'>{t('connects all')}</span>
          <span className='home-title-line home-title-accent'>
            {t('frontier AI models')}
          </span>
        </h1>
        <p className='home-hero-sub'>
          <span className='home-hero-sub-lead'>
            {t('Built for AI applications, global developers, and teams')}
          </span>
          <span className='home-hero-sub-muted'>
            <Trans
              i18nKey='Fast direct access, no account-ban risk, non-expiring balance, and better value. Low-latency access to ChatGPT, Claude, Gemini, and other frontier models.'
              components={{
                nowrap: <span className='home-cjk-nowrap' />,
              }}
            />
          </span>
        </p>
        <div className='home-actions'>
          {props.isAuthenticated ? (
            <Link className='home-btn home-btn-primary' to='/dashboard'>
              {t('Go to Dashboard')}
              <ArrowRight aria-hidden='true' className='home-btn-arrow' />
            </Link>
          ) : (
            <>
              <Link className='home-btn home-btn-primary' to='/sign-up'>
                {t('Get Started')}
                <ArrowRight aria-hidden='true' className='home-btn-arrow' />
              </Link>
              <Link className='home-btn' to='/pricing'>
                {t('View Pricing')}
              </Link>
            </>
          )}
        </div>
        <div className='home-extension-apps'>
          <div className='home-extension-apps-label'>
            <span>{t('Extension apps by AI Cove')}</span>
          </div>
          <div
            className='home-extension-app-actions'
            role='group'
            aria-label={t('Extension apps by AI Cove')}
          >
            <a
              className='home-btn home-extension-app-button'
              data-download-platform={desktopDownload.platform}
              data-testid='home-desktop-download'
              download
              href={desktopDownload.href}
              aria-label={t(desktopDownload.ariaLabelKey)}
            >
              <span aria-hidden='true' className='home-extension-app-icon'>
                <img
                  alt=''
                  className='home-desktop-download-icon'
                  src='/desgin-logo.png'
                />
              </span>
              <span className='home-extension-app-copy'>
                <strong>AI Cove Design</strong>
                <span>
                  {t('Create AI images on canvas')} · {desktopPlatformLabel}
                </span>
              </span>
              <Download aria-hidden='true' className='home-btn-arrow' />
            </a>
            <a
              className='home-btn home-extension-app-button'
              data-download-platform={turboDownload.platform}
              data-testid='home-turbo-download'
              download
              href={turboDownload.href}
              aria-label={t(turboDownload.ariaLabelKey)}
            >
              <span aria-hidden='true' className='home-extension-app-icon'>
                <img
                  alt=''
                  className='home-desktop-download-icon'
                  src='/turbo-icon.png'
                />
              </span>
              <span className='home-extension-app-copy'>
                <strong>AI Cove Turbo</strong>
                <span>
                  {t('OpenAI model acceleration engine')} ·{' '}
                  {desktopPlatformLabel}
                </span>
              </span>
              <Download aria-hidden='true' className='home-btn-arrow' />
            </a>
          </div>
        </div>
      </div>
      <div
        className='landing-animate-fade-up'
        style={{ animationDelay: '180ms' }}
      >
        <HeroTerminalDemo />
      </div>
    </section>
  )
}
