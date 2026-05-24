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
import { AnimateInView } from '@/components/animate-in-view'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  if (props.isAuthenticated) {
    return null
  }

  return (
    <section className='home-section home-shell' id='pricing'>
      <AnimateInView className='home-cta-panel' animation='scale-in'>
        <h2>
          {t('Ready to strengthen')}
          <br />
          <em>{t('your AI application?')}</em>
        </h2>
        <p>
          {t(
            'Sign up, create a key, check pricing, paste the Base URL into your project: first 200 OK in four steps.'
          )}
        </p>
        <div className='home-actions'>
          <Link className='home-btn home-btn-primary' to='/sign-up'>
            {t('Get Started')}
            <ArrowRight aria-hidden='true' className='home-btn-arrow' />
          </Link>
          <Link className='home-btn' to='/pricing'>
            {t('View Pricing')}
          </Link>
        </div>
      </AnimateInView>
    </section>
  )
}
