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
import { getRouteApi } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { ModelRoutingWorkspace } from '@/features/model-routing'

import { MatrixWorkspace } from './components/matrix-workspace'

const route = getRouteApi('/_authenticated/model-channel-matrix/')

export function ModelChannelMatrix() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [view, setView] = useState<'pools' | 'matrix'>('pools')
  const [dirty, setDirty] = useState(false)
  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Model routing')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-3'>
          <div
            className='flex shrink-0 gap-2'
            role='group'
            aria-label={t('Routing view')}
          >
            <Button
              variant={view === 'pools' ? 'default' : 'outline'}
              size='sm'
              disabled={dirty && view !== 'pools'}
              onClick={() => setView('pools')}
            >
              {t('Routing pools')}
            </Button>
            <Button
              variant={view === 'matrix' ? 'default' : 'outline'}
              size='sm'
              disabled={dirty}
              onClick={() => setView('matrix')}
            >
              {t('Channel defaults matrix')}
            </Button>
          </div>
          {view === 'pools' ? (
            <ModelRoutingWorkspace onDirtyChange={setDirty} />
          ) : (
            <MatrixWorkspace
              search={search}
              onSearchChange={(next) => {
                void navigate({ search: next })
              }}
            />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
