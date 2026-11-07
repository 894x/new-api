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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  useComboboxAnchor,
} from '@/components/ui/combobox'
import { searchUsers } from '@/features/users/api'
import { useDebounce } from '@/hooks'
import {
  canAccessUserGroupConfig,
  hasPermission,
  ADMIN_PERMISSION_RESOURCES,
  ADMIN_PERMISSION_ACTIONS,
} from '@/lib/admin-permissions'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

export function RoutingScopeSelect(props: {
  userId?: number
  allUsers?: boolean
  disabled: boolean
  onChange: (userId?: number, allUsers?: boolean) => void
}) {
  const { t } = useTranslation()
  const actor = useAuthStore((s) => s.auth.user)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const keyword = useDebounce(search, 250)
  const anchor = useComboboxAnchor()
  const canSearch =
    hasPermission(
      actor,
      ADMIN_PERMISSION_RESOURCES.USER,
      ADMIN_PERMISSION_ACTIONS.READ
    ) &&
    hasPermission(
      actor,
      ADMIN_PERMISSION_RESOURCES.USER_GROUP_CONFIG,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const users = useQuery({
    queryKey: ['routing-user-options', keyword],
    queryFn: async () =>
      requireServerSuccess(await searchUsers({ keyword, p: 1, page_size: 20 }))
        .data?.items ?? [],
    enabled: canSearch && open && !props.disabled,
    staleTime: 30000,
    meta: { errorToast: false },
  })
  const canReadDefaults = hasPermission(
    actor,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const options = [
    ...(canReadDefaults
      ? [{ value: '__default', label: t('Default resource pool') }]
      : []),
    ...(canReadDefaults && canSearch
      ? [{ value: '__all', label: t('All') }]
      : []),
    ...(users.data ?? [])
      .filter(
        (user) => !user.DeletedAt && canAccessUserGroupConfig(actor, user.role)
      )
      .map((user) => ({
        value: String(user.id),
        label: `${user.username} · #${user.id}`,
      })),
  ]
  let value = '__default'
  if (props.allUsers) value = '__all'
  if (props.userId) value = String(props.userId)
  const selected = options.find((option) => option.value === value) ?? {
    value,
    label: t('User #{{id}}', { id: props.userId }),
  }
  return (
    <Combobox<{ value: string; label: string }>
      items={options}
      value={selected}
      disabled={props.disabled}
      open={open && !props.disabled}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        setSearch('')
      }}
      inputValue={open ? search : selected.label}
      onInputValueChange={(text, details) => {
        if (details.reason === 'input-change') setSearch(text)
      }}
      filter={null}
      itemToStringLabel={(option) => option.label}
      isItemEqualToValue={(a, b) => a.value === b.value}
      onValueChange={(option) => {
        if (!option || props.disabled) return
        if (option.value === '__default') props.onChange()
        else if (option.value === '__all') props.onChange(undefined, true)
        else props.onChange(Number(option.value))
        setOpen(false)
        setSearch('')
      }}
    >
      <div ref={anchor} className='h-8 min-w-40 flex-1 sm:max-w-64'>
        <ComboboxInput
          aria-label={t('Select routing user')}
          placeholder={t('Search users by name or ID...')}
          className='h-full min-h-8'
          disabled={props.disabled}
        />
      </div>
      <ComboboxContent anchor={anchor}>
        <ComboboxList>
          {options.map((option) => (
            <ComboboxItem key={option.value} value={option}>
              {option.label}
            </ComboboxItem>
          ))}
          {users.isFetching && (
            <p role='status' className='p-3 text-sm'>
              {t('Loading users...')}
            </p>
          )}
          {users.isError && (
            <Button variant='ghost' onClick={() => void users.refetch()}>
              {t('Retry')}
            </Button>
          )}
          {canSearch &&
            !users.isFetching &&
            !users.isError &&
            !users.data?.length && (
              <p className='p-3 text-sm'>{t('No users found')}</p>
            )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
