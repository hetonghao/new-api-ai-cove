import { type ReactNode, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  BadgePercent,
  CheckCircle2,
  Flame,
  HandCoins,
  type LucideIcon,
  ReceiptText,
  RefreshCw,
  Search,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getSalesCommissionSettlementsByRoot } from '../api'
import {
  canCreateCommissionSettlement,
  calculateSettlementAmountByPercent,
  getSettlementAmountError,
  roundMoney,
  SETTLEMENT_PERCENTAGES,
  shouldShowSettlementForm,
  type SettlementDialogMode,
  type SettlementPercentage,
} from '../lib/commission-settlement'
import type {
  SalesCommissionAdminRow,
  SalesCommissionSettlement,
  SalesStats,
} from '../types'

const ROOT_SETTLEMENT_HISTORY_SIZE = 5

function formatYuanAmount(amount: number | null | undefined): string {
  return `¥${formatNumber(amount ?? 0)}`
}

function formatYuanPairAmount(amount: number | null | undefined): string {
  return `${formatNumber(amount ?? 0)}¥`
}

function formatRatio(ratio: number | null | undefined): string {
  return `${formatNumber(ratio ?? 0)}%`
}

function getSalespersonName(row: SalesCommissionAdminRow): string {
  return row.display_name || row.username
}

function createEmptySettlementPage(page: number, pageSize: number) {
  return { items: [], total: 0, page, page_size: pageSize }
}

function Metric({
  icon: Icon,
  label,
  value,
  valueClassName,
}: {
  icon: LucideIcon
  label: string
  value: ReactNode
  valueClassName?: string
}) {
  return (
    <div className='border-border/80 bg-card/40 flex min-h-24 items-center gap-3 rounded-md border px-4 py-3'>
      <div className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-md'>
        <Icon className='size-4' />
      </div>
      <div className='min-w-0'>
        <div className='text-muted-foreground text-xs'>{label}</div>
        <div
          className={cn(
            'truncate text-xl font-semibold tabular-nums',
            valueClassName
          )}
        >
          {value}
        </div>
      </div>
    </div>
  )
}

function AmountPairValue({
  primaryAmount,
  secondaryAmount,
  primaryClassName,
  secondaryClassName,
}: {
  primaryAmount: number | null | undefined
  secondaryAmount: number | null | undefined
  primaryClassName: string
  secondaryClassName: string
}) {
  return (
    <>
      <span className={primaryClassName}>
        {formatYuanPairAmount(primaryAmount)}
      </span>
      <span className='text-muted-foreground'> / </span>
      <span className={secondaryClassName}>
        {formatYuanPairAmount(secondaryAmount)}
      </span>
    </>
  )
}

function SettlementStateIcon({
  icon: Icon,
  label,
  className,
}: {
  icon: LucideIcon
  label: string
  className: string
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            aria-label={label}
            className={cn('ml-1.5 inline-flex align-middle', className)}
          />
        }
      >
        <Icon className='size-4' />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function CommissionSettlementsTable({
  settlements,
  isLoading,
  emptyLabel,
}: {
  settlements: SalesCommissionSettlement[]
  isLoading: boolean
  emptyLabel: string
}) {
  const { t } = useTranslation()

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Settlement Time')}</TableHead>
          <TableHead className='text-right'>{t('Settlement Amount')}</TableHead>
          <TableHead className='text-right'>{t('Ratio Snapshot')}</TableHead>
          <TableHead className='text-right'>{t('Covered Revenue')}</TableHead>
          <TableHead>{t('Note')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {settlements.length === 0 ? (
          <TableRow>
            <TableCell
              colSpan={5}
              className='text-muted-foreground h-24 text-center'
            >
              {isLoading ? t('Loading...') : emptyLabel}
            </TableCell>
          </TableRow>
        ) : (
          settlements.map((settlement) => (
            <TableRow key={settlement.id}>
              <TableCell>
                {formatTimestampToDate(settlement.created_at)}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatYuanAmount(settlement.amount)}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatRatio(settlement.commission_ratio)}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatYuanAmount(settlement.covered_revenue)}
              </TableCell>
              <TableCell className='max-w-64 truncate'>
                {settlement.note || '-'}
              </TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  )
}

export function SalesCommissionSettlementsTab({
  stats,
  settlements,
  isLoading,
  isFetching,
  page,
  pageCount,
  onPreviousPage,
  onNextPage,
  onRefresh,
}: {
  stats: SalesStats
  settlements: SalesCommissionSettlement[]
  isLoading: boolean
  isFetching: boolean
  page: number
  pageCount: number
  onPreviousPage: () => void
  onNextPage: () => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const totalCommissionAmount = stats.total_commission_amount ?? 0
  const settledCommissionAmount = stats.settled_commission_amount ?? 0
  const pendingCommissionRevenue = stats.pending_commission_revenue ?? 0
  const pendingCommissionAmount = stats.pending_commission_amount ?? 0
  const hasTotalCommission = totalCommissionAmount > 0
  const hasPendingSettlement =
    roundMoney(pendingCommissionRevenue) > 0 ||
    roundMoney(pendingCommissionAmount) > 0
  const fullySettled =
    hasTotalCommission &&
    roundMoney(totalCommissionAmount - settledCommissionAmount) <= 0
  const noPendingSettlement = !hasPendingSettlement

  return (
    <>
      <div className='grid gap-3 md:grid-cols-3'>
        <Metric
          icon={ReceiptText}
          label={t('Total / Settled Commission')}
          value={
            <>
              <AmountPairValue
                primaryAmount={stats.total_commission_amount}
                secondaryAmount={stats.settled_commission_amount}
                primaryClassName='text-foreground'
                secondaryClassName='text-teal-600 dark:text-teal-400'
              />
              {fullySettled && (
                <SettlementStateIcon
                  icon={CheckCircle2}
                  label={t('Fully Settled')}
                  className='text-emerald-600 dark:text-emerald-400'
                />
              )}
            </>
          }
        />
        <Metric
          icon={HandCoins}
          label={t('Pending Revenue / Commission')}
          value={
            <>
              <AmountPairValue
                primaryAmount={stats.pending_commission_revenue}
                secondaryAmount={stats.pending_commission_amount}
                primaryClassName='text-foreground'
                secondaryClassName='text-amber-600 dark:text-amber-400'
              />
              {noPendingSettlement && (
                <SettlementStateIcon
                  icon={Flame}
                  label={t('Keep Creating Glory')}
                  className='text-emerald-600 dark:text-emerald-400'
                />
              )}
            </>
          }
        />
        <Metric
          icon={BadgePercent}
          label={t('Commission Ratio')}
          value={formatRatio(stats.commission_ratio)}
          valueClassName='text-indigo-600 dark:text-indigo-400'
        />
      </div>

      <section className='border-border/80 rounded-md border'>
        <div className='flex items-center justify-between border-b p-3'>
          <div className='font-medium'>{t('Settlement Records')}</div>
          <Button
            type='button'
            variant='outline'
            onClick={onRefresh}
            disabled={isFetching}
          >
            <RefreshCw className='size-4' />
            {t('Refresh')}
          </Button>
        </div>
        <CommissionSettlementsTable
          settlements={settlements}
          isLoading={isLoading}
          emptyLabel={t('No commission settlements found')}
        />
        <div className='flex items-center justify-between border-t px-3 py-2'>
          <div className='text-muted-foreground text-sm'>
            {t('Page')} {page} / {pageCount}
          </div>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={onPreviousPage}
            >
              {t('Previous Page')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page >= pageCount}
              onClick={onNextPage}
            >
              {t('Next Page')}
            </Button>
          </div>
        </div>
      </section>
    </>
  )
}

export function RootCommissionSettlementsTab({
  rows,
  isLoading,
  isFetching,
  keyword,
  page,
  pageCount,
  ratioDrafts,
  onKeywordChange,
  onRefresh,
  onPreviousPage,
  onNextPage,
  onRatioDraftChange,
  onUpdateRatio,
  onOpenDetails,
  onOpenSettlement,
  updatingSalesUserId,
}: {
  rows: SalesCommissionAdminRow[]
  isLoading: boolean
  isFetching: boolean
  keyword: string
  page: number
  pageCount: number
  ratioDrafts: Record<number, string>
  onKeywordChange: (value: string) => void
  onRefresh: () => void
  onPreviousPage: () => void
  onNextPage: () => void
  onRatioDraftChange: (salesUserId: number, value: string) => void
  onUpdateRatio: (row: SalesCommissionAdminRow) => void
  onOpenDetails: (row: SalesCommissionAdminRow) => void
  onOpenSettlement: (row: SalesCommissionAdminRow) => void
  updatingSalesUserId?: number
}) {
  const { t } = useTranslation()

  return (
    <section className='border-border/80 rounded-md border'>
      <div className='flex flex-col gap-3 border-b p-3 md:flex-row md:items-center md:justify-between'>
        <div className='relative min-w-0 flex-1 md:max-w-sm'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
          <Input
            value={keyword}
            onChange={(event) => onKeywordChange(event.target.value)}
            placeholder={t('Search sales users')}
            className='pl-8'
          />
        </div>
        <Button
          type='button'
          variant='outline'
          onClick={onRefresh}
          disabled={isFetching}
        >
          <RefreshCw className='size-4' />
          {t('Refresh')}
        </Button>
      </div>

      <div className='overflow-x-auto'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Salesperson')}</TableHead>
              <TableHead className='text-right'>{t('Total Revenue')}</TableHead>
              <TableHead>{t('Commission Ratio')}</TableHead>
              <TableHead className='text-right'>
                {t('Settled Commission')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Pending Revenue')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Pending Commission')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Total Commission')}
              </TableHead>
              <TableHead>{t('Last Settlement')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className='text-muted-foreground h-28 text-center'
                >
                  {isLoading ? t('Loading...') : t('No sales users found')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => {
                const ratioDraft =
                  ratioDrafts[row.sales_user_id] ??
                  String(row.commission_ratio ?? 0)
                const isUpdating = updatingSalesUserId === row.sales_user_id
                const displayContact = row.email || row.username
                const canCreateSettlement = canCreateCommissionSettlement(
                  row.pending_commission_amount,
                  row.commission_ratio
                )
                return (
                  <TableRow key={row.sales_user_id}>
                    <TableCell>
                      <div className='font-medium'>
                        {getSalespersonName(row)}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {displayContact}
                      </div>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatYuanAmount(row.total_revenue)}
                    </TableCell>
                    <TableCell>
                      <div className='flex min-w-40 items-center gap-2'>
                        <Input
                          type='number'
                          min='0'
                          max='100'
                          step='0.01'
                          value={ratioDraft}
                          onChange={(event) =>
                            onRatioDraftChange(
                              row.sales_user_id,
                              event.target.value
                            )
                          }
                          className='h-7 w-20'
                        />
                        <span className='text-muted-foreground text-sm'>%</span>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => onUpdateRatio(row)}
                          disabled={isUpdating}
                        >
                          {t('Save')}
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatYuanAmount(row.settled_commission_amount)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatYuanAmount(row.pending_commission_revenue)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatYuanAmount(row.pending_commission_amount)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatYuanAmount(row.total_commission_amount)}
                    </TableCell>
                    <TableCell>
                      {formatTimestampToDate(row.last_settlement_created_at)}
                    </TableCell>
                    <TableCell className='text-right'>
                      <div className='flex justify-end gap-2'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => onOpenDetails(row)}
                        >
                          <ReceiptText className='size-3.5' />
                          {t('Details')}
                        </Button>
                        <Button
                          type='button'
                          size='sm'
                          onClick={() => onOpenSettlement(row)}
                          disabled={!canCreateSettlement}
                        >
                          <HandCoins className='size-3.5' />
                          {t('Settle')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <div className='flex items-center justify-between border-t px-3 py-2'>
        <div className='text-muted-foreground text-sm'>
          {t('Page')} {page} / {pageCount}
        </div>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page <= 1}
            onClick={onPreviousPage}
          >
            {t('Previous Page')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page >= pageCount}
            onClick={onNextPage}
          >
            {t('Next Page')}
          </Button>
        </div>
      </div>
    </section>
  )
}

export function SettlementDialog({
  row,
  mode,
  open,
  isSubmitting,
  onOpenChange,
  onSubmit,
}: {
  row: SalesCommissionAdminRow | null
  mode: SettlementDialogMode
  open: boolean
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (payload: {
    salesUserId: number
    amount: number
    note: string
  }) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        {row ? (
          <SettlementDialogForm
            key={row.sales_user_id}
            row={row}
            mode={mode}
            isSubmitting={isSubmitting}
            onOpenChange={onOpenChange}
            onSubmit={onSubmit}
          />
        ) : (
          <DialogHeader>
            <DialogTitle>{t('Create Settlement')}</DialogTitle>
            <DialogDescription>
              {t('Select a salesperson first')}
            </DialogDescription>
          </DialogHeader>
        )}
      </DialogContent>
    </Dialog>
  )
}

function SettlementDialogForm({
  row,
  mode,
  isSubmitting,
  onOpenChange,
  onSubmit,
}: {
  row: SalesCommissionAdminRow
  mode: SettlementDialogMode
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (payload: {
    salesUserId: number
    amount: number
    note: string
  }) => void
}) {
  const { t } = useTranslation()
  const canCreateSettlement = canCreateCommissionSettlement(
    row.pending_commission_amount,
    row.commission_ratio
  )
  const showSettlementForm = shouldShowSettlementForm(mode, canCreateSettlement)
  const [inputMode, setInputMode] = useState<'percent' | 'manual'>('percent')
  const [selectedPercent, setSelectedPercent] =
    useState<SettlementPercentage>(100)
  const [amountInput, setAmountInput] = useState(() =>
    String(
      calculateSettlementAmountByPercent(
        row.pending_commission_amount ?? 0,
        100
      )
    )
  )
  const [note, setNote] = useState('')
  const pendingAmount = row.pending_commission_amount ?? 0
  const amount = Number(amountInput)
  const amountError = getSettlementAmountError(amount, pendingAmount)
  const remainingAmount = roundMoney(
    Math.max(pendingAmount - (Number.isFinite(amount) ? amount : 0), 0)
  )

  const historyQuery = useQuery({
    queryKey: ['sales-commission-settlements-root', row.sales_user_id, t],
    queryFn: async () => {
      const result = await getSalesCommissionSettlementsByRoot(
        row.sales_user_id,
        {
          p: 1,
          page_size: ROOT_SETTLEMENT_HISTORY_SIZE,
        }
      )
      if (!result.success) {
        toast.error(
          result.message || t('Failed to load commission settlements')
        )
        return createEmptySettlementPage(1, ROOT_SETTLEMENT_HISTORY_SIZE)
      }
      return (
        result.data ||
        createEmptySettlementPage(1, ROOT_SETTLEMENT_HISTORY_SIZE)
      )
    },
  })

  const handlePercentClick = (percent: SettlementPercentage) => {
    setInputMode('percent')
    setSelectedPercent(percent)
    setAmountInput(
      String(calculateSettlementAmountByPercent(pendingAmount, percent))
    )
  }

  const handleSubmit = () => {
    if (amountError) return
    onSubmit({
      salesUserId: row.sales_user_id,
      amount: roundMoney(amount),
      note: note.trim(),
    })
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>
          {mode === 'settle' ? t('Create Settlement') : t('Commission Details')}
        </DialogTitle>
        <DialogDescription>
          {`${getSalespersonName(row)} - ${t('Pending Commission')} ${formatYuanAmount(pendingAmount)}`}
        </DialogDescription>
      </DialogHeader>

      <div className='space-y-4'>
        <div className='grid gap-3 sm:grid-cols-3'>
          <Metric
            icon={BadgePercent}
            label={t('Commission Ratio')}
            value={formatRatio(row.commission_ratio)}
          />
          <Metric
            icon={Wallet}
            label={t('Pending Revenue')}
            value={formatYuanAmount(row.pending_commission_revenue)}
          />
          <Metric
            icon={HandCoins}
            label={t('Pending Commission')}
            value={formatYuanAmount(row.pending_commission_amount)}
          />
        </div>

        {showSettlementForm ? (
          <>
            <div className='space-y-2'>
              <Label>{t('Settlement percentage')}</Label>
              <div className='flex flex-wrap gap-2'>
                {SETTLEMENT_PERCENTAGES.map((percent) => (
                  <Button
                    key={percent}
                    type='button'
                    variant={
                      inputMode === 'percent' && selectedPercent === percent
                        ? 'default'
                        : 'outline'
                    }
                    onClick={() => handlePercentClick(percent)}
                  >
                    {percent}%
                  </Button>
                ))}
                <Button
                  type='button'
                  variant={inputMode === 'manual' ? 'default' : 'outline'}
                  onClick={() => setInputMode('manual')}
                >
                  {t('Manual amount')}
                </Button>
              </div>
            </div>

            <div className='grid gap-3 sm:grid-cols-[1fr_14rem]'>
              <div className='space-y-2'>
                <Label htmlFor='commission-settlement-amount'>
                  {t('Settlement amount')}
                </Label>
                <Input
                  id='commission-settlement-amount'
                  type='number'
                  min='0'
                  step='0.01'
                  value={amountInput}
                  onChange={(event) => {
                    setInputMode('manual')
                    setAmountInput(event.target.value)
                  }}
                />
                {amountError && (
                  <p className='text-destructive text-sm'>
                    {amountError === 'too-large'
                      ? t('Settlement amount cannot exceed pending commission')
                      : t('Settlement amount must be greater than 0')}
                  </p>
                )}
              </div>
              <div className='border-border/80 flex flex-col justify-center rounded-md border px-3 py-2'>
                <div className='text-muted-foreground text-xs'>
                  {t('Remaining after settlement')}
                </div>
                <div className='text-lg font-semibold tabular-nums'>
                  {formatYuanAmount(remainingAmount)}
                </div>
              </div>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='commission-settlement-note'>{t('Note')}</Label>
              <Textarea
                id='commission-settlement-note'
                value={note}
                onChange={(event) => setNote(event.target.value)}
                placeholder={t('Optional settlement note')}
              />
            </div>
          </>
        ) : canCreateSettlement ? null : (
          <div className='text-muted-foreground rounded-md border px-3 py-2 text-sm'>
            {t('No pending commission available')}
          </div>
        )}

        <div className='space-y-2'>
          <div className='font-medium'>{t('Recent Settlements')}</div>
          <div className='max-h-64 overflow-auto rounded-md border'>
            <CommissionSettlementsTable
              settlements={historyQuery.data?.items || []}
              isLoading={historyQuery.isLoading}
              emptyLabel={t('No settlement records')}
            />
          </div>
        </div>
      </div>

      <DialogFooter>
        <Button
          type='button'
          variant='outline'
          onClick={() => onOpenChange(false)}
        >
          {t('Cancel')}
        </Button>
        {showSettlementForm && (
          <Button
            type='button'
            onClick={handleSubmit}
            disabled={Boolean(amountError) || isSubmitting}
          >
            <HandCoins className='size-4' />
            {t('Create Settlement')}
          </Button>
        )}
      </DialogFooter>
    </>
  )
}
