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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Label } from '@/components/ui/label'
import { canAccessUserGroupConfig } from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUserRoutingPool,
  getUserServiceTypes,
  routingPoolKeys,
  saveUserServiceType,
  serviceTypeKeys,
  userRoutingPoolKeys,
  type RoutingServiceType,
  type UserServiceTypeBinding,
} from './api'

export function UserServiceTypeSettings(props: {
  userId: number
  model?: string
  group?: string
  readOnly?: boolean
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [model, setModel] = useState('')
  const [group, setGroup] = useState('')
  const actor = useAuthStore((state) => state.auth.user)
  const query = useQuery({
    queryKey: serviceTypeKeys.user(props.userId),
    queryFn: () => getUserServiceTypes(props.userId),
    enabled: open,
    meta: { errorToast: false },
  })
  const catalog = useQuery({
    queryKey: userRoutingPoolKeys.catalog(props.userId),
    queryFn: () => getUserRoutingPool(props.userId),
    enabled: open,
    meta: { errorToast: false },
  })
  const canWrite =
    !props.readOnly &&
    !!catalog.data &&
    canAccessUserGroupConfig(actor, catalog.data.user_role, 'write')
  const bindings = query.data?.bindings ?? []
  const scoped = bindings.filter((row) => row.model || row.routing_group)
  const defaultBinding = bindings.find(
    (row) => !row.model && !row.routing_group
  )
  const accountTypeName = query.data?.service_types.find(
    (row) => row.id === defaultBinding?.service_type_id
  )?.name
  const identities = new Map(
    scoped.map((row) => [
      `${row.model}\u0000${row.routing_group}`,
      { model: row.model, group: row.routing_group },
    ])
  )
  if (props.model && props.group) {
    identities.set(`${props.model}\u0000${props.group}`, {
      model: props.model,
      group: props.group,
    })
  }
  if (model || group) identities.set(`${model}\u0000${group}`, { model, group })
  return (
    <>
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={props.disabled}
        aria-label={t('Service type')}
        onClick={() => setOpen(true)}
      >
        {t('Service type')}
        {!props.model && !props.group && accountTypeName && (
          <span className='max-w-40 truncate'>{accountTypeName}</span>
        )}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Routing service types')}
        description={t(
          'Choose an account default and add model or public group exceptions. User-specific channel pools take precedence.'
        )}
      >
        {(query.isPending || catalog.isPending) && <LoadingState />}
        {(query.isError || catalog.isError) && (
          <ErrorState
            onRetry={() => {
              void query.refetch()
              void catalog.refetch()
            }}
          />
        )}
        {query.data && catalog.data && (
          <div className='space-y-4'>
            <UserServiceTypeSelect
              userId={props.userId}
              model=''
              group=''
              label={t('Account service type')}
              binding={defaultBinding}
              types={query.data.service_types}
              disabled={!canWrite}
            />
            <div className='space-y-3 border-t pt-4'>
              <p className='text-sm font-medium'>
                {t('Service type exceptions')}
              </p>
              {[...identities.entries()].map(([key, scope]) => (
                <div key={key} className='min-w-0 rounded-lg border p-3'>
                  <UserServiceTypeSelect
                    userId={props.userId}
                    model={scope.model}
                    group={scope.group}
                    label={`${scope.model || t('All models')} · ${scope.group || t('All groups')}`}
                    binding={bindings.find(
                      (row) =>
                        row.model === scope.model &&
                        row.routing_group === scope.group
                    )}
                    types={query.data.service_types}
                    disabled={!canWrite}
                  />
                </div>
              ))}
              {canWrite && (
                <div className='grid gap-2 sm:grid-cols-2'>
                  <Combobox
                    aria-label={t('Model exception')}
                    placeholder={t('All models')}
                    allowCustomValue
                    maxLength={255}
                    value={model}
                    options={[
                      { value: '', label: t('All models') },
                      ...catalog.data.models.map((value) => ({
                        value,
                        label: value,
                      })),
                    ]}
                    onValueChange={(value) => setModel(value ?? '')}
                  />
                  <Combobox
                    aria-label={t('Public group exception')}
                    placeholder={t('All groups')}
                    value={group}
                    options={[
                      { value: '', label: t('All groups') },
                      ...Object.entries(catalog.data.key_groups ?? {}).map(
                        ([value, label]) => ({ value, label })
                      ),
                    ]}
                    onValueChange={(value) => setGroup(value ?? '')}
                  />
                </div>
              )}
            </div>
          </div>
        )}
      </Dialog>
    </>
  )
}

function UserServiceTypeSelect(props: {
  userId: number
  model: string
  group: string
  label: string
  binding?: UserServiceTypeBinding
  types: RoutingServiceType[]
  disabled: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const client = useQueryClient()
  const account = !props.model && !props.group
  const save = useMutation({
    mutationFn: (value: number) => {
      if (props.disabled) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveUserServiceType(props.userId, {
        model: props.model,
        routing_group: props.group,
        service_type_id: Math.max(0, value),
        revision: props.binding?.revision ?? '',
        reset: value < 0 || (account && value === 0),
      })
    },
    onSuccess: async () => {
      toast.success(t('Saved successfully'))
      await Promise.all([
        client.invalidateQueries({
          queryKey: serviceTypeKeys.user(props.userId),
        }),
        client.invalidateQueries({
          queryKey: userRoutingPoolKeys.all(props.userId),
        }),
        client.invalidateQueries({ queryKey: routingPoolKeys.all }),
        client.invalidateQueries({
          queryKey: ['user-channel-routing', props.userId],
        }),
        client.invalidateQueries({ queryKey: ['routing-monitor-pool'] }),
      ])
    },
    onError: async (error) => {
      handleServerError(error, t('Failed to save'))
      await client.invalidateQueries({
        queryKey: serviceTypeKeys.user(props.userId),
      })
    },
  })
  let value = props.binding?.service_type_id ?? (account ? 0 : -1)
  if (save.isPending) value = save.variables
  return (
    <div className='space-y-2'>
      <Label htmlFor={id} className='break-all'>
        {props.label}
      </Label>
      <Combobox
        id={id}
        aria-label={props.label}
        value={String(value)}
        disabled={props.disabled || save.isPending}
        options={[
          ...(!account
            ? [{ value: '-1', label: t('Inherit service type') }]
            : []),
          { value: '0', label: t('Default routing pool') },
          ...props.types.map((row) => ({
            value: String(row.id),
            label: row.name,
          })),
        ]}
        onValueChange={(next) => {
          if (next !== null && Number(next) !== value) save.mutate(Number(next))
        }}
      />
      {save.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Changes were not saved. Reload if another administrator changed this configuration.'
          )}
        </p>
      )}
    </div>
  )
}
