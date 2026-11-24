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

import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'

import { DeviMonMonitor } from './components/devi-mon-monitor'
import { DeviMonSettingsPanel } from './components/devi-mon-settings'

export function DeviMonPage(props: { readonly tabs: ReactNode }) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Subscription operations')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>{props.tabs}</SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {settingsOpen ? (
          <DeviMonSettingsPanel onClose={() => setSettingsOpen(false)} />
        ) : (
          <DeviMonMonitor onOpenSettings={() => setSettingsOpen(true)} />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
