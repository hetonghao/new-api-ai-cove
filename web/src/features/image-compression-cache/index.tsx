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
import { useTranslation } from 'react-i18next'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CacheConfiguration } from './configuration'
import { CacheStatistics } from './statistics'

export function ImageCompressionCache() {
  const { t } = useTranslation()
  return (
    <Tabs defaultValue='configuration' className='min-w-0 gap-4'>
      <TabsList variant='line' aria-label={t('Image compression cache')}>
        <TabsTrigger value='configuration'>{t('Configuration')}</TabsTrigger>
        <TabsTrigger value='statistics'>
          {t('imageCompressionCache.statistics', {
            defaultValue: 'Statistics',
          })}
        </TabsTrigger>
      </TabsList>
      <TabsContent value='configuration'>
        <CacheConfiguration />
      </TabsContent>
      <TabsContent value='statistics'>
        <CacheStatistics />
      </TabsContent>
    </Tabs>
  )
}
