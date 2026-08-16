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
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'

import { useSystemConfig } from '@/hooks/use-system-config'

import { HomeFooterStrands } from './home-footer-strands'

const AI_COVE_DESIGN_DESCRIPTION =
  'Visual creation powered by the same models, keys, and billing.'
const COVE_TURBO_DESCRIPTION =
  'Local acceleration and connection visibility for faster, steadier Codex sessions.'
const AI_COVE_DESCRIPTION = 'Models, keys, routing, and billing in one gateway.'

type ProductCardProps = {
  readonly product: 'design' | 'turbo'
}

function ProductCard(props: ProductCardProps): ReactElement {
  const { t } = useTranslation()
  const isTurbo = props.product === 'turbo'
  const productSuffix = isTurbo ? 'Turbo' : 'Design'

  return (
    <article
      className={`home-footer-product-card home-footer-product-card--${props.product}`}
    >
      <img
        src={isTurbo ? '/turbo-icon.png' : '/desgin-logo.png'}
        alt=''
        className='home-footer-product-logo'
        aria-hidden='true'
      />
      <div className='home-footer-product-copy'>
        <span>
          {t(isTurbo ? 'Model acceleration engine' : 'Canvas image workspace')}
        </span>
        <strong
          className={`ai-cove-design-wordmark home-footer-product-wordmark${isTurbo ? ' home-footer-turbo-wordmark' : ''}`}
          aria-label={`AI Cove ${productSuffix}`}
        >
          <span className='ai-cove-design-wordmark__prefix' aria-hidden='true'>
            AI
          </span>
          <span
            className='ai-cove-design-wordmark__space ai-cove-design-wordmark__space--after-prefix'
            aria-hidden='true'
          />
          <span
            className='home-footer-product-wordmark__cove'
            aria-hidden='true'
          >
            Cove
          </span>
          <span className='ai-cove-design-wordmark__space' aria-hidden='true' />
          <span
            className='home-footer-product-wordmark__suffix'
            aria-hidden='true'
          >
            {productSuffix}
          </span>
        </strong>
        <p>
          {t(isTurbo ? COVE_TURBO_DESCRIPTION : AI_COVE_DESIGN_DESCRIPTION)}
        </p>
      </div>
    </article>
  )
}

export function HomeFooter(): ReactElement {
  const { t } = useTranslation()
  const { logo } = useSystemConfig()
  const currentYear = new Date().getFullYear()
  const displayName = 'AI Cove'
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
            <p>{t(AI_COVE_DESCRIPTION)}</p>
          </div>
          <div className='home-footer-connection' aria-hidden='true'>
            <HomeFooterStrands />
          </div>
          <div className='home-footer-product-stack'>
            <ProductCard product='design' />
            <ProductCard product='turbo' />
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
