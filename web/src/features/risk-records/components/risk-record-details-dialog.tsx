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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { getAllLogs } from '@/features/usage-logs/api'
import { DetailsDialog as UsageLogDetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import {
  usageLogSchema,
  type UsageLog,
} from '@/features/usage-logs/data/schema'
import { formatDateTimeStr } from '@/lib/format'

import type { RiskRecord } from '../types'
import {
  RiskRecordCategoryList,
  RiskRecordChannelSummary,
  RiskRecordChunkList,
  RiskRecordIdList,
  RiskRecordLatency,
  RiskRecordProviderSummary,
  RiskRecordResultBadge,
  RiskRecordSourceBadge,
  RiskRecordTokenSummary,
  RiskRecordUserSummary,
} from './risk-record-summary'

function DetailRow(props: {
  readonly label: string
  readonly value: ReactNode
}) {
  return (
    <div className='grid min-w-0 grid-cols-[7rem_minmax(0,1fr)] gap-3 text-sm max-sm:grid-cols-1 max-sm:gap-1'>
      <dt className='text-muted-foreground'>{props.label}</dt>
      <dd className='min-w-0 break-words'>{props.value}</dd>
    </div>
  )
}

function DetailSection(props: {
  readonly title: string
  readonly children: ReactNode
}) {
  return (
    <section className='min-w-0 space-y-2'>
      <h3 className='text-sm font-semibold'>{props.title}</h3>
      <dl className='bg-muted/30 min-w-0 space-y-2 rounded-md border p-3'>
        {props.children}
      </dl>
    </section>
  )
}

function RiskRecordRequestDetailsButton(props: { readonly requestId: string }) {
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
      const response = await getAllLogs({
        p: 1,
        page_size: 1,
        request_id: props.requestId,
      })
      const parsed = usageLogSchema.safeParse(response.data?.items[0])
      if (!response.success || !parsed.success) {
        toast.error(t('No matching usage record'))
        return
      }
      setUsageLog(parsed.data)
      setOpen(true)
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load request details:', error)
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
      {usageLog && (
        <UsageLogDetailsDialog
          log={usageLog}
          isAdmin
          open={open}
          onOpenChange={setOpen}
        />
      )}
    </>
  )
}

function RiskRecordDetailsDialog(props: {
  readonly record: RiskRecord
  readonly open: boolean
  readonly onOpenChange: (open: boolean) => void
  readonly onUserClick: (record: RiskRecord) => void
}) {
  const { t } = useTranslation()
  const { record } = props

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Risk record details')}
      description={formatDateTimeStr(new Date(record.observed_at))}
      contentClassName='sm:max-w-2xl'
      bodyClassName='space-y-4'
      showCloseButton
    >
      {record.preview && (
        <DetailSection title={t('Redacted detection content')}>
          <pre className='font-sans text-sm leading-relaxed break-words whitespace-pre-wrap'>
            {record.preview}
          </pre>
        </DetailSection>
      )}
      <DetailSection title={t('Review details')}>
        <DetailRow
          label={t('Result')}
          value={<RiskRecordResultBadge result={record.result} />}
        />
        <DetailRow
          label={t('Source')}
          value={<RiskRecordSourceBadge source={record.source} />}
        />
        <DetailRow
          label={t('Provider')}
          value={<RiskRecordProviderSummary record={record} />}
        />
        <DetailRow
          label={t('Categories')}
          value={<RiskRecordCategoryList values={record.categories} />}
        />
        <DetailRow
          label={t('Cloud call')}
          value={record.provider_called ? t('Yes') : t('No')}
        />
        <DetailRow
          label={t('Cache hit')}
          value={record.cache_hit ? t('Yes') : t('No')}
        />
        <DetailRow
          label={t('Blocked')}
          value={record.blocked ? t('Yes') : t('No')}
        />
        {record.error_code && (
          <DetailRow
            label={t('Error')}
            value={
              <span className='text-destructive'>{record.error_code}</span>
            }
          />
        )}
      </DetailSection>
      <DetailSection title={t('Request information')}>
        <DetailRow
          label={t('Request ID')}
          value={
            <RiskRecordRequestDetailsButton requestId={record.request_id} />
          }
        />
        <DetailRow
          label={t('Channel')}
          value={<RiskRecordChannelSummary record={record} />}
        />
        <DetailRow
          label={t('User')}
          value={
            <RiskRecordUserSummary
              record={record}
              onClick={props.onUserClick}
            />
          }
        />
        {record.token_id > 0 && (
          <DetailRow
            label={t('API token')}
            value={<RiskRecordTokenSummary record={record} />}
          />
        )}
        {record.model && <DetailRow label={t('Model')} value={record.model} />}
        {record.rule_ids.length > 0 && (
          <DetailRow
            label={t('Rules')}
            value={<RiskRecordIdList values={record.rule_ids} />}
          />
        )}
        {record.content_hash && (
          <DetailRow
            label={t('Content hash')}
            value={
              <span
                className='block truncate font-mono'
                title={record.content_hash}
              >
                {record.content_hash}
              </span>
            }
          />
        )}
      </DetailSection>
      <DetailSection title={t('Token usage')}>
        <DetailRow
          label={t('Latency')}
          value={<RiskRecordLatency latencyMs={record.latency_ms} />}
        />
        <DetailRow
          label={t('Prompt')}
          value={record.prompt_tokens.toLocaleString()}
        />
        <DetailRow
          label={t('Completion')}
          value={record.completion_tokens.toLocaleString()}
        />
        <DetailRow
          label={t('Total tokens')}
          value={record.total_tokens.toLocaleString()}
        />
        <DetailRow
          label={t('Neurons')}
          value={record.neurons.toLocaleString()}
        />
      </DetailSection>
      {record.chunks.length > 0 && (
        <section className='min-w-0 space-y-2'>
          <h3 className='text-sm font-semibold'>{t('Cloud call details')}</h3>
          <RiskRecordChunkList chunks={record.chunks} />
        </section>
      )}
    </Dialog>
  )
}

export function RiskRecordDetailsButton(props: {
  readonly record: RiskRecord
  readonly onUserClick: (record: RiskRecord) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button
        type='button'
        variant='link'
        size='sm'
        className='h-auto max-w-full justify-start px-0 text-xs tabular-nums'
        onClick={() => setOpen(true)}
      >
        {t('{{count}} tokens', { count: props.record.total_tokens })}
      </Button>
      <RiskRecordDetailsDialog
        record={props.record}
        open={open}
        onOpenChange={setOpen}
        onUserClick={props.onUserClick}
      />
    </>
  )
}
