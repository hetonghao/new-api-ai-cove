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
import type { ColumnDef } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { toIntlLocale } from '@/i18n/languages'
import { formatBytes, formatNumber, formatTimestampToDate } from '@/lib/format'

import type { CacheEntry } from './api'

export function useEntryColumns(
  remove: (ids: readonly string[]) => void,
  disabled: boolean
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return useMemo<ColumnDef<CacheEntry, unknown>[]>(
    () => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            aria-label={t('Select all')}
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(checked) =>
              table.toggleAllPageRowsSelected(checked)
            }
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            aria-label={t('Select row {{number}}', { number: row.index + 1 })}
            checked={row.getIsSelected()}
            onCheckedChange={(checked) => row.toggleSelected(checked)}
          />
        ),
      },
      {
        accessorKey: 'id',
        header: t('Entry ID'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <span
            className='block max-w-36 truncate font-mono'
            title={row.original.id}
          >
            {row.original.id}
          </span>
        ),
      },
      { accessorKey: 'user_id', header: t('User ID') },
      {
        id: 'image_format',
        header: t('Image format'),
        cell: ({ row }) => (
          <span className='whitespace-normal'>
            {row.original.original_mime.replace('image/', '')} →{' '}
            {row.original.compressed_mime.replace('image/', '')}
          </span>
        ),
      },
      {
        id: 'image_size',
        header: t('Original / compressed'),
        cell: ({ row }) => (
          <span className='whitespace-normal tabular-nums'>
            {formatBytes(row.original.original_bytes, locale)} /{' '}
            {formatBytes(row.original.compressed_bytes, locale)}
          </span>
        ),
      },
      {
        accessorKey: 'hits',
        header: t('Hits'),
        cell: ({ row }) => formatNumber(row.original.hits, locale),
      },
      {
        accessorKey: 'created_at',
        header: t('Created At'),
        cell: ({ row }) => formatTimestampToDate(row.original.created_at),
      },
      {
        accessorKey: 'last_hit_at',
        header: t('Last hit'),
        cell: ({ row }) =>
          formatTimestampToDate(row.original.last_hit_at ?? undefined),
      },
      {
        accessorKey: 'expires_at',
        header: t('Expires at'),
        cell: ({ row }) => formatTimestampToDate(row.original.expires_at),
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('Clear cache entry {{id}}', { id: row.original.id })}
            disabled={disabled}
            onClick={() => remove([row.original.id])}
          >
            <Trash2 aria-hidden='true' />
          </Button>
        ),
      },
    ],
    [disabled, locale, remove, t]
  )
}
