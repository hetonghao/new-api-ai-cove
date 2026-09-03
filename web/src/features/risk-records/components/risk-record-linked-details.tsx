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
import { ExternalLink, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { DetailsDialog as UsageLogDetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import type { UsageLog } from '@/features/usage-logs/data/schema'

import { getRiskRequestLog } from '../api'

export function RiskRecordRequestDetailsButton(props: {
  readonly requestId: string
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [usageLog, setUsageLog] = useState<UsageLog | null>(null)
  const [open, setOpen] = useState(false)

  async function openUsageLog() {
    if (usageLog) {
      setOpen(true)
      return
    }

    setLoading(true)
    try {
      const response = await getRiskRequestLog(props.requestId)
      if (!response.success || !response.data) {
        toast.error(t('No matching usage record'))
        return
      }
      setUsageLog(response.data)
      setOpen(true)
    } catch {
      toast.error(t('Failed to load request details'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      <Button
        type='button'
        variant='link'
        size='sm'
        className='h-auto max-w-full min-w-0 justify-start gap-1 px-0 font-mono text-sm'
        disabled={loading}
        onClick={() => void openUsageLog()}
      >
        <span className='min-w-0 truncate' title={props.requestId}>
          {props.requestId}
        </span>
        {loading ? (
          <Loader2 className='size-3.5 shrink-0 animate-spin' />
        ) : (
          <ExternalLink className='size-3.5 shrink-0' />
        )}
      </Button>
      {usageLog ? (
        <UsageLogDetailsDialog
          log={usageLog}
          isAdmin
          isRoot={false}
          open={open}
          onOpenChange={setOpen}
        />
      ) : null}
    </>
  )
}

export function RiskRecordErrorDetailsButton(props: {
  readonly errorCode: string
  readonly errorDetail?: string | null
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const label = props.errorCode || t('Error')
  const detail = props.errorDetail?.trim()

  return (
    <>
      <Button
        type='button'
        variant='link'
        size='sm'
        className='text-warning h-auto max-w-full justify-start px-0 text-sm'
        onClick={() => setOpen(true)}
      >
        <span className='min-w-0 truncate' title={label}>
          {label}
        </span>
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Error details')}
        description={label}
        contentClassName='sm:max-w-lg'
        bodyClassName='min-w-0'
        showCloseButton
      >
        {detail ? (
          <pre className='bg-muted/30 max-h-[50dvh] overflow-y-auto rounded-md border p-3 font-mono text-sm break-words whitespace-pre-wrap'>
            {detail}
          </pre>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('No error details available')}
          </p>
        )}
      </Dialog>
    </>
  )
}
