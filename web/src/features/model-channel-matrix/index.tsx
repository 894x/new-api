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
import { RoutingScopeSelect } from '@/features/model-routing/scope-select'
import { UserRoutingWorkspace } from '@/features/model-routing/user-workspace'

import { GroupModelWorkspace } from './components/group-model-workspace'
import { MatrixWorkspace } from './components/matrix-workspace'

const route = getRouteApi('/_authenticated/model-channel-matrix/')

export function ModelChannelMatrix() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [view, setView] = useState<'pools' | 'matrix' | 'groups'>('pools')
  const [dirty, setDirty] = useState(false)
  const scopeSelect = (
    <RoutingScopeSelect
      userId={search.user_id}
      allUsers={search.pool_scope === 'all'}
      disabled={dirty}
      onChange={(userId, allUsers) => {
        if (dirty) return
        setView('pools')
        void navigate({
          search: (previous) => ({
            ...previous,
            user_id: userId,
            pool_scope: allUsers ? 'all' : undefined,
          }),
        })
      }}
    />
  )
  let workspace = (
    <ModelRoutingWorkspace
      allUsers={search.pool_scope === 'all'}
      scopeSelect={scopeSelect}
      onDirtyChange={setDirty}
    />
  )
  if (search.user_id) {
    workspace = (
      <UserRoutingWorkspace
        key={search.user_id}
        userId={search.user_id}
        onDirtyChange={setDirty}
        scopeSelect={scopeSelect}
      />
    )
  } else if (view === 'groups') {
    workspace = <GroupModelWorkspace onDirtyChange={setDirty} />
  } else if (view === 'matrix') {
    workspace = (
      <MatrixWorkspace
        search={search}
        onSearchChange={(next) => {
          void navigate({ search: next })
        }}
      />
    )
  }
  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Model routing')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-3'>
          {!search.user_id && view !== 'pools' && (
            <div className='flex flex-wrap gap-2'>{scopeSelect}</div>
          )}
          {!search.user_id && (
            <div
              className='flex shrink-0 flex-wrap gap-2'
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
                variant={view === 'groups' ? 'default' : 'outline'}
                size='sm'
                disabled={dirty && view !== 'groups'}
                onClick={() => setView('groups')}
              >
                {t('Group-model matrix')}
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
          )}
          {workspace}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
