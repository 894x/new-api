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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { routingPoolKeys } from '@/features/model-routing/api'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getGroupModelMatrix,
  groupModelMatrixKey,
  saveGroupModelMatrix,
  type GroupModelAccess,
} from '../group-model-api'
import { GroupModelTable } from './group-model-table'
import { MatrixPagination } from './matrix-pagination'

export function GroupModelWorkspace(props: {
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const canRead =
    (user?.role ?? 0) >= ROLE.ADMIN &&
    hasPermission(
      user,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const canWrite =
    canRead &&
    hasPermission(
      user,
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_ACTIONS.WRITE
    )
  const client = useQueryClient()
  const [draft, setDraft] = useState<{
    config: GroupModelAccess
    revision: string
  } | null>(null)
  const [groupFilter, setGroupFilter] = useState('')
  const [modelFilter, setModelFilter] = useState('')
  const [groupPage, setGroupPage] = useState(1)
  const [modelPage, setModelPage] = useState(1)
  const dirty = draft !== null
  const notifyDirty = useEffectEvent(props.onDirtyChange)
  useEffect(() => {
    notifyDirty(dirty)
  }, [dirty])
  useEffect(() => () => notifyDirty(false), [])
  const query = useQuery({
    queryKey: groupModelMatrixKey,
    queryFn: getGroupModelMatrix,
    enabled: canRead,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const save = useMutation({
    mutationFn: () => {
      if (!draft) throw new Error('No pending matrix changes')
      return saveGroupModelMatrix(draft.config, draft.revision)
    },
    onSuccess: (result) => {
      client.setQueryData(groupModelMatrixKey, { ...query.data, ...result })
      setDraft(null)
      void client.invalidateQueries({ queryKey: routingPoolKeys.all })
      void client.invalidateQueries({ queryKey: ['model-routing-catalog'] })
      void client.invalidateQueries({ queryKey: ['user-routing-pools'] })
      toast.success(t('Saved successfully'))
    },
  })
  if (!canRead) {
    return <EmptyState title={t("You don't have necessary permission")} />
  }
  if (query.isPending) return <Skeleton className='min-h-32 flex-1' />
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  const data = query.data
  const config = draft?.config ?? data.config
  const groups = data.groups.filter((group) =>
    group.toLowerCase().includes(groupFilter.toLowerCase())
  )
  const models = data.models.filter((model) =>
    model.toLowerCase().includes(modelFilter.toLowerCase())
  )
  const currentGroupPage = Math.min(
    groupPage,
    Math.max(1, Math.ceil(groups.length / 20))
  )
  const currentModelPage = Math.min(
    modelPage,
    Math.max(1, Math.ceil(models.length / 10))
  )
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col gap-3'>
      <FormNavigationGuard when={dirty} />
      <p className='text-muted-foreground text-sm'>
        {t(
          'Limit the models each group can use across all channels. Unrestricted groups inherit all channel models; restricted groups only allow selected models, including for future additions.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Channel group membership is still required. Empty selections block all models. Model modifiers follow base model access. Existing routing pool settings are preserved when access is disabled.'
        )}
      </p>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          className='w-full sm:w-56'
          aria-label={t('Search groups')}
          placeholder={t('Search groups')}
          value={groupFilter}
          onChange={(event) => {
            setGroupFilter(event.target.value)
            setGroupPage(1)
          }}
        />
        <Input
          className='w-full sm:w-64'
          aria-label={t('Search models')}
          placeholder={t('Search models')}
          value={modelFilter}
          onChange={(event) => {
            setModelFilter(event.target.value)
            setModelPage(1)
          }}
        />
        <div className='flex flex-1 justify-end gap-2'>
          <Button
            variant='outline'
            disabled={save.isPending || (!dirty && query.isFetching)}
            onClick={() => {
              setDraft(null)
              void query.refetch()
            }}
          >
            {dirty ? t('Discard changes') : t('Refresh')}
          </Button>
          {canWrite && (
            <Button
              disabled={!dirty || save.isPending}
              onClick={() => save.mutate()}
            >
              {save.isPending ? t('Saving...') : t('Save')}
            </Button>
          )}
        </div>
      </div>
      {groups.length > 0 && models.length > 0 ? (
        <GroupModelTable
          data={data}
          groups={groups.slice(
            (currentGroupPage - 1) * 20,
            currentGroupPage * 20
          )}
          models={models.slice(
            (currentModelPage - 1) * 10,
            currentModelPage * 10
          )}
          config={config}
          disabled={!canWrite || save.isPending}
          onChange={(group, selected) => {
            setDraft((previous) => {
              let next = { ...(previous?.config ?? data.config) }
              if (selected === null) delete next[group]
              else next = { ...next, [group]: selected }
              return {
                config: next,
                revision: previous?.revision ?? data.revision,
              }
            })
          }}
        />
      ) : (
        <EmptyState title={t('No Data')} />
      )}
      <div className='flex shrink-0 flex-wrap items-center justify-between gap-3'>
        <MatrixPagination
          axis='groups'
          page={currentGroupPage}
          pageSize={20}
          total={groups.length}
          disabled={false}
          onPageChange={setGroupPage}
        />
        <MatrixPagination
          axis='models'
          page={currentModelPage}
          pageSize={10}
          total={models.length}
          disabled={false}
          onPageChange={setModelPage}
        />
      </div>
    </div>
  )
}
