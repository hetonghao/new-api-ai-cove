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
import {
  Code2,
  PanelTop,
  Sparkles,
  Terminal,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'

interface FeaturesProps {
  className?: string
}

interface EcosystemTool {
  num: string
  vendor: string
  title: string
  description: string
  badge: string
  toneSoft: string
  toneDark: string
  icon: LucideIcon
  signature: 'terminal' | 'thinking' | 'code' | 'diamond'
}

interface ProofRow {
  num: string
  title: string
  description: string
  tone: string
}

function EcosystemSignature(props: { type: EcosystemTool['signature'] }) {
  if (props.type === 'terminal') {
    return (
      <div className='eco-signature'>
        <div className='sig-terminal' aria-hidden='true'>
          <div className='rows'>
            <div className='row'>
              <span className='prompt'>›</span>
              <span className='cmd'>chat &quot;refactor this module&quot;</span>
            </div>
            <div className='row'>
              <span className='prompt'>›</span>
              <span className='out'>3 files · </span>
              <span className='ok'>done</span>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (props.type === 'thinking') {
    return (
      <div className='eco-signature'>
        <span className='sig-thinking' aria-hidden='true'>
          <span className='pulse' />
          Extended Thinking
        </span>
      </div>
    )
  }

  if (props.type === 'code') {
    return (
      <div className='eco-signature'>
        <span className='sig-code' aria-hidden='true'>
          <span className='bracket'>{'{'}</span>
          <span className='dots'>···</span>
          <span className='bracket'>{'}'}</span>
        </span>
      </div>
    )
  }

  return (
    <div className='eco-signature'>
      <span className='sig-diamond' aria-hidden='true'>
        <span className='diamond' />
        <span className='diamond small' />
        <span className='label'>Gemini</span>
      </span>
    </div>
  )
}

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  const ecosystemTools: EcosystemTool[] = [
    {
      num: '01 / 04',
      vendor: 'Open Source',
      title: 'OpenClaw',
      description: t(
        'Open-source local AI assistant that can execute tasks on your computer through chat, not just respond.'
      ),
      badge: t('Open source · local runtime'),
      toneSoft: 'var(--home-peach-soft)',
      toneDark: '#8a5a2a',
      icon: Sparkles,
      signature: 'terminal',
    },
    {
      num: '02 / 04',
      vendor: 'Anthropic',
      title: 'Claude Code',
      description: t(
        'Anthropic official CLI with Extended Thinking support for code work that needs deeper reasoning.'
      ),
      badge: t('Anthropic official'),
      toneSoft: 'var(--home-sage-soft)',
      toneDark: '#5f895c',
      icon: Terminal,
      signature: 'thinking',
    },
    {
      num: '03 / 04',
      vendor: 'OpenAI',
      title: 'Codex',
      description: t(
        'OpenAI coding agent for larger refactors, bug fixes, and tests across real repositories.'
      ),
      badge: t('OpenAI official'),
      toneSoft: 'var(--home-sky-soft)',
      toneDark: '#2d6e89',
      icon: Code2,
      signature: 'code',
    },
    {
      num: '04 / 04',
      vendor: 'Google',
      title: 'Gemini CLI',
      description: t(
        'Google open-source terminal agent for coding, debugging, and workflow automation from the command line.'
      ),
      badge: t('Google open source'),
      toneSoft: 'var(--home-lavender-soft)',
      toneDark: '#6e5e9e',
      icon: PanelTop,
      signature: 'diamond',
    },
  ]

  const proofRows: ProofRow[] = [
    {
      num: '01',
      title: t('Direct access'),
      description: t(
        'Reach the gateway consistently from local tools and CI without waiting on brittle upstream paths.'
      ),
      tone: '#8a6f2a',
    },
    {
      num: '02',
      title: t('Highly available routing'),
      description: t(
        'Health checks, fallback order, and grouped channels keep critical requests moving.'
      ),
      tone: '#5f895c',
    },
    {
      num: '03',
      title: t('Simple integration'),
      description: t(
        'Change the Base URL, keep your client code, and route through OpenAI-compatible APIs.'
      ),
      tone: '#2d6e89',
    },
    {
      num: '04',
      title: t('Full compatibility'),
      description: t(
        'Preserve common model behaviors across Chat, Responses, Claude, Gemini, and image workflows.'
      ),
      tone: '#6e5e9e',
    },
  ]

  return (
    <section className='home-section home-shell home-features' id='features'>
      <AnimateInView className='home-section-head'>
        <h2>
          {t('Built for developers,')}
          <br />
          <em>{t('designed for scale')}</em>
        </h2>
      </AnimateInView>

      <AnimateInView className='home-core-block'>
        <div className='ecosystem-head'>
          <div>
            <div className='home-kicker'>{t('Compatible ecosystem')}</div>
            <h3>{t('Supported devices and AI coding tools')}</h3>
          </div>
          <p className='note'>
            {t(
              'One Base URL connects local assistants, official CLIs, and coding agents to the same gateway.'
            )}
          </p>
        </div>

        <div className='ecosystem-sheet'>
          <div
            className='ecosystem-grid'
            aria-label={t('Compatible ecosystem')}
          >
            {ecosystemTools.map((tool) => {
              const Icon = tool.icon
              return (
                <article
                  className='eco-tool'
                  key={tool.title}
                  style={
                    {
                      '--tone-soft': tool.toneSoft,
                      '--tone-dark': tool.toneDark,
                    } as React.CSSProperties
                  }
                >
                  <div className='eco-corner'>
                    <span className='num'>{tool.num}</span>
                    <span className='vendor'>{tool.vendor}</span>
                  </div>
                  <span className='eco-symbol' aria-hidden='true'>
                    <Icon />
                  </span>
                  <h4>{tool.title}</h4>
                  <p>{tool.description}</p>
                  <EcosystemSignature type={tool.signature} />
                  <span className='eco-badge'>{tool.badge}</span>
                </article>
              )
            })}
          </div>
        </div>
      </AnimateInView>

      <hr className='home-core-rule' aria-hidden='true' />

      <AnimateInView className='home-core-block'>
        <div className='proof-spread'>
          <header className='proof-head'>
            <div className='home-kicker'>{t('Usage value')}</div>
            <h3>
              {t(
                'Unlock your coding throughput and let frontier AI write with you'
              )}
            </h3>
            <p className='lede'>
              {t('Four reasons developers ship faster on AI-Cove.')}
            </p>
          </header>

          <ol className='proof-list' aria-label={t('Usage value')}>
            {proofRows.map((row) => (
              <li
                className='proof-row'
                key={row.num}
                style={{ '--tone-dark': row.tone } as React.CSSProperties}
              >
                <span className='row-num'>{row.num}</span>
                <div className='row-content'>
                  <div className='row-line'>
                    <strong>{row.title}</strong>
                    <span className='row-leader' aria-hidden='true' />
                  </div>
                  <p>{row.description}</p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </AnimateInView>
    </section>
  )
}
