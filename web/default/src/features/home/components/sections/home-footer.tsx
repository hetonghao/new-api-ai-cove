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
import { useTranslation } from 'react-i18next'
import { useSystemConfig } from '@/hooks/use-system-config'

const AI_COVE_DESIGN_DESCRIPTION =
  'AI Cove hosts a canvas-based image creation subsystem that is efficient, convenient, cost-effective, and connected to the same model, key, and billing foundation.'
const FOOTER_PULSE_PATH =
  'M3 20H30L42 12L52 28L65 6L78 34L92 10L106 24L118 16L130 20H165'

export function HomeFooter() {
  const { t } = useTranslation()
  const { logo } = useSystemConfig()
  const currentYear = new Date().getFullYear()
  const displayName = 'AI-Cove'
  const displayLogo = logo || '/logo.png'

  return (
    <footer className='home-footer'>
      <div className='home-shell'>
        <div className='home-footer-main'>
          <div className='home-footer-platform'>
            <Link to='/' className='home-footer-brand'>
              <img src={displayLogo} alt={displayName} />
              <span>
                <small>{t('Unified API Gateway')}</small>
                <strong>{displayName}</strong>
              </span>
            </Link>
            <p>{t('Powerful API Management Platform')}</p>
          </div>
          <div className='home-footer-system-pulse' aria-hidden='true'>
            <svg viewBox='0 0 168 40' focusable='false'>
              <path d={FOOTER_PULSE_PATH} />
            </svg>
          </div>
          <div className='home-footer-design-card'>
            <div className='home-footer-design-rail' aria-hidden='true'>
              <span />
              <span />
            </div>
            <img
              src='/desgin-logo.png'
              alt=''
              className='home-footer-design-logo'
              aria-hidden='true'
            />
            <div className='home-footer-design-copy'>
              <span>{t('Integrated subsystem')}</span>
              <strong>{t('AI-Cove-Design')}</strong>
              <p>{t(AI_COVE_DESIGN_DESCRIPTION)}</p>
            </div>
          </div>
        </div>
        <div className='home-footer-bottom'>
          <span>
            © {currentYear} AI Cove · {t('footer.aiCoveRights')}
          </span>
          <span className='home-footer-tagline'>
            <span className='home-footer-tagline-mark' aria-hidden='true' />
            {t('One key · many model providers · usage-based billing')}
          </span>
        </div>
      </div>
    </footer>
  )
}
