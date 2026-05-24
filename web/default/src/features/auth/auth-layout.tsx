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
import Antigravity from '@lobehub/icons/es/Antigravity'
import CherryStudio from '@lobehub/icons/es/CherryStudio'
import ClaudeCode from '@lobehub/icons/es/ClaudeCode'
import Codex from '@lobehub/icons/es/Codex'
import Gemini from '@lobehub/icons/es/Gemini'
import NousResearch from '@lobehub/icons/es/NousResearch'
import OpenClaw from '@lobehub/icons/es/OpenClaw'
import OpenCode from '@lobehub/icons/es/OpenCode'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useSystemConfig } from '@/hooks/use-system-config'
import { FloatingLines } from '@/components/backgrounds/floating-lines'
import { Skeleton } from '@/components/ui/skeleton'

type AuthLayoutProps = {
  children: React.ReactNode
  variant?: 'default' | 'home'
}

type AuthMarqueeLabel = {
  label: string
  Icon?: React.ElementType<{ size?: number | string; 'aria-hidden'?: boolean }>
  iconClassName?: string
}

function ZedIcon({
  size = 14,
  ...props
}: React.SVGProps<SVGSVGElement> & { size?: number | string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox='0 0 24 24'
      fill='currentColor'
      focusable='false'
      {...props}
    >
      <path d='M2.25 1.5a.75.75 0 0 0-.75.75v16.5H0V2.25A2.25 2.25 0 0 1 2.25 0h20.095c1.002 0 1.504 1.212.795 1.92L10.764 14.298h3.486V12.75h1.5v1.922a1.125 1.125 0 0 1-1.125 1.125H9.264l-2.578 2.578h11.689V9h1.5v9.375a1.5 1.5 0 0 1-1.5 1.5H5.185L2.562 22.5H21.75a.75.75 0 0 0 .75-.75V5.25H24v16.5A2.25 2.25 0 0 1 21.75 24H1.655C.653 24 .151 22.788.86 22.08L13.19 9.75H9.75v1.5h-1.5V9.375A1.125 1.125 0 0 1 9.375 8.25h5.314l2.625-2.625H5.625V15h-1.5V5.625a1.5 1.5 0 0 1 1.5-1.5h13.19L21.438 1.5z' />
    </svg>
  )
}

const AUTH_MARQUEE_LABELS = [
  { label: 'Codex', Icon: Codex.Color },
  { label: 'OpenClaw', Icon: OpenClaw.Color },
  {
    label: 'Hermes',
    Icon: NousResearch,
    iconClassName: 'ai-cove-auth-proof-icon-hermes',
  },
  { label: 'Claude Code', Icon: ClaudeCode.Color },
  { label: 'Gemini CLI', Icon: Gemini.Color },
  { label: 'antigravity', Icon: Antigravity.Color },
  { label: 'Cherry Studio', Icon: CherryStudio.Color },
  {
    label: 'Opencode',
    Icon: OpenCode,
    iconClassName: 'ai-cove-auth-proof-icon-opencode',
  },
  {
    label: 'Zed',
    Icon: ZedIcon,
    iconClassName: 'ai-cove-auth-proof-icon-zed',
  },
  { label: '高性价比' },
] satisfies AuthMarqueeLabel[]
const AUTH_MARQUEE_CYCLES = [0, 1]
const AUTH_FLOATING_WAVES: Array<'top' | 'middle' | 'bottom'> = ['middle']
const AUTH_LINE_GRADIENT = ['#1a9fd3', '#7d95b7', '#6a6a6a']

export function AuthLayout({ children, variant = 'default' }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const isHomeVariant = variant === 'home'
  const displayName = isHomeVariant ? 'AI-Cove' : systemName
  const [oneApiSitePrefix, oneApiSiteSuffix] = t('One API site').split('API')

  return (
    <div
      className={cn(
        'relative grid h-svh max-w-none',
        isHomeVariant && 'ai-cove-auth-shell'
      )}
    >
      {isHomeVariant && (
        <div className='ai-cove-auth-atmosphere' aria-hidden='true'>
          <FloatingLines
            animationSpeed={3}
            bendRadius={8}
            bendStrength={-2}
            className='ai-cove-auth-floating-field'
            enabledWaves={AUTH_FLOATING_WAVES}
            interactive={false}
            lineCount={5}
            lineDistance={23}
            linesGradient={AUTH_LINE_GRADIENT}
            mixBlendMode='normal'
            parallax={false}
          />
        </div>
      )}
      <Link
        to='/'
        className={cn(
          'absolute top-4 left-4 z-10 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8',
          isHomeVariant && 'ai-cove-auth-brand'
        )}
      >
        <div className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className={cn(
                'h-8 w-8 object-cover',
                isHomeVariant ? 'rounded-xl' : 'rounded-full'
              )}
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{displayName}</h1>
        )}
      </Link>
      <div
        className={cn(
          'container flex items-center pt-16 sm:pt-0',
          isHomeVariant && 'ai-cove-auth-stage'
        )}
      >
        {isHomeVariant && (
          <aside className='ai-cove-auth-story' aria-label='AI Cove'>
            <div className='ai-cove-auth-kicker'>
              {t('Unified API Gateway')}
            </div>
            <h2>
              <span className='ai-cove-auth-title-line'>
                {oneApiSitePrefix}
                <span className='ai-cove-auth-title-api'>{t('API')}</span>
                {oneApiSiteSuffix}
              </span>
              <span className='ai-cove-auth-title-line'>
                {t('connects all')}
              </span>
              <span className='ai-cove-auth-title-line ai-cove-auth-title-accent'>
                {t('frontier AI models')}
              </span>
            </h2>
            <p>
              {t('Built for AI applications, global developers, and teams')}
            </p>
            <div className='ai-cove-auth-proof' aria-hidden='true'>
              {AUTH_MARQUEE_CYCLES.map((cycle) => (
                <div className='ai-cove-auth-proof-track' key={cycle}>
                  {AUTH_MARQUEE_LABELS.map(({ label, Icon, iconClassName }) => (
                    <span
                      className={cn(
                        'ai-cove-auth-proof-pill',
                        Icon && 'ai-cove-auth-proof-pill-with-icon'
                      )}
                      key={`${cycle}-${label}`}
                    >
                      {Icon && (
                        <span
                          className={cn(
                            'ai-cove-auth-proof-icon',
                            iconClassName
                          )}
                        >
                          <Icon size={14} aria-hidden='true' />
                        </span>
                      )}
                      {label}
                    </span>
                  ))}
                </div>
              ))}
            </div>
          </aside>
        )}
        <div
          className={cn(
            'mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8',
            isHomeVariant && 'ai-cove-auth-card'
          )}
        >
          {children}
        </div>
      </div>
    </div>
  )
}
