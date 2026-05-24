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
import { Activity, KeyRound, PlugZap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'

export function HowItWorks() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '01',
      title: t('Configure'),
      desc: t(
        'Add your API keys, set up channels and configure access permissions'
      ),
      toneSoft: 'var(--home-peach-soft)',
      toneLine: 'rgba(245, 184, 133, 0.55)',
      tone: '#8a5a2a',
      icon: <KeyRound />,
    },
    {
      num: '02',
      title: t('Connect'),
      desc: t(
        'Connect through OpenAI, Claude, Gemini, and other compatible API routes'
      ),
      toneSoft: 'var(--home-sky-soft)',
      toneLine: 'rgba(179, 212, 232, 0.6)',
      tone: '#2d6e89',
      icon: <PlugZap />,
    },
    {
      num: '03',
      title: t('Monitor'),
      desc: t('Track usage, costs and performance with real-time analytics'),
      toneSoft: 'var(--home-sage-soft)',
      toneLine: 'rgba(158, 199, 158, 0.6)',
      tone: '#5f895c',
      icon: <Activity />,
    },
  ]

  return (
    <section
      className='home-section home-shell'
      id='how-it-works'
      aria-labelledby='how-title'
    >
      <AnimateInView className='home-section-head center'>
        <div className='home-kicker'>{t('How It Works')}</div>
        <h2 id='how-title'>{t('Three steps to get started')}</h2>
      </AnimateInView>

      <div className='home-steps-flow'>
        {steps.map((step, i) => (
          <AnimateInView
            key={step.num}
            delay={i * 120}
            className='home-step-card'
            style={
              {
                '--step-soft': step.toneSoft,
                '--step-line': step.toneLine,
                '--step-tone': step.tone,
              } as React.CSSProperties
            }
          >
            <span className='step-num-display' aria-hidden='true'>
              {step.num}
            </span>
            <div className='step-icon' aria-hidden='true'>
              {step.icon}
            </div>
            <span className='step-tag'>
              <span className='step-tag-num'>Step {step.num}</span>
            </span>
            <h3>{step.title}</h3>
            <p>{step.desc}</p>
          </AnimateInView>
        ))}
      </div>
    </section>
  )
}
