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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { HeroFloatingLines } from '../hero-floating-lines'
import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const [oneApiSitePrefix, oneApiSiteSuffix] = t('One API site').split('API')

  return (
    <section className='home-shell home-hero' aria-labelledby='home-hero-title'>
      <HeroFloatingLines />
      <div className='home-hero-copy landing-animate-fade-up'>
        <div className='home-eyebrow'>AI-Cove</div>
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
            {t(
              'Fast direct access, no account-ban risk, non-expiring balance, and better value. Low-latency access to ChatGPT, Claude, Gemini, and other frontier models.'
            )}
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
