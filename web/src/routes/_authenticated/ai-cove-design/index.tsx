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
import { useEffect, useMemo, useRef } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { useTheme } from '@/context/theme-provider'
import { createAiCoveDesignSidecarUrl } from '@/lib/ai-cove-sidecar-url'
import { isSidebarModuleEnabled } from '@/lib/nav-modules'
import { Main } from '@/components/layout'

const HOST_CREDENTIALS_MESSAGE_TYPE = 'ai-cove-design.host-credentials'
const HOST_READY_MESSAGE_TYPE = 'ai-cove-design.ready'

export const Route = createFileRoute('/_authenticated/ai-cove-design/')({
  beforeLoad: () => {
    if (!isSidebarModuleEnabled('chat', 'playground')) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: AiCoveDesignPage,
})

function AiCoveDesignPage() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const accessToken = useAuthStore((state) => state.auth.accessToken)
  const { resolvedTheme } = useTheme()
  const initialThemeRef = useRef(resolvedTheme)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const sidecarUrl = useMemo(
    () =>
      userId ? createAiCoveDesignSidecarUrl(userId, initialThemeRef.current) : '',
    [userId]
  )

  useEffect(() => {
    if (!accessToken || !userId || !sidecarUrl) return

    const sidecarOrigin = new URL(sidecarUrl).origin
    if (sidecarOrigin === window.location.origin) return

    const postCredentials = () =>
      iframeRef.current?.contentWindow?.postMessage(
        { type: HOST_CREDENTIALS_MESSAGE_TYPE, token: accessToken, userId },
        sidecarOrigin
      )
    const receiveReady = (event: MessageEvent<unknown>) => {
      if (
        event.source !== iframeRef.current?.contentWindow ||
        event.origin !== sidecarOrigin ||
        typeof event.data !== 'object' ||
        event.data === null ||
        !('type' in event.data) ||
        event.data.type !== HOST_READY_MESSAGE_TYPE
      ) {
        return
      }
      postCredentials()
    }

    window.addEventListener('message', receiveReady)
    postCredentials()
    return () => window.removeEventListener('message', receiveReady)
  }, [accessToken, sidecarUrl, userId])

  if (!userId) {
    return (
      <Main className='bg-background p-0'>
        <div
          data-testid='ai-cove-design-loading'
          className='text-muted-foreground flex size-full items-center justify-center text-sm'
        >
          正在加载 AI Cove Design...
        </div>
      </Main>
    )
  }

  return (
    <Main className='bg-background p-0'>
      <iframe
        ref={iframeRef}
        title='AI Cove Design'
        src={sidecarUrl}
        className='bg-background size-full border-0'
        allow='clipboard-read; clipboard-write'
      />
    </Main>
  )
}
