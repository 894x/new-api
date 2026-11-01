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
import type { User } from '@/features/users/types'
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
  disabled: boolean
  onChange: (userId?: number) => void
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
  const options = (users.data ?? []).filter(
    (user) => !user.DeletedAt && canAccessUserGroupConfig(actor, user.role)
  )
  return (
    <div className='flex min-w-0 flex-wrap items-center gap-2'>
      <span className='text-muted-foreground text-sm'>
        {t('Configuration target')}
      </span>
      <Button
        size='sm'
        variant={!props.userId ? 'default' : 'outline'}
        disabled={
          props.disabled ||
          !hasPermission(
            actor,
            ADMIN_PERMISSION_RESOURCES.CHANNEL,
            ADMIN_PERMISSION_ACTIONS.READ
          )
        }
        onClick={() => props.onChange()}
      >
        {t('Default configuration')}
      </Button>
      {canSearch && (
        <Combobox<User>
          items={options}
          value={null}
          open={open && !props.disabled}
          onOpenChange={setOpen}
          inputValue={search}
          onInputValueChange={setSearch}
          filter={null}
          itemToStringLabel={(user) => `${user.username} #${user.id}`}
          isItemEqualToValue={(a, b) => a.id === b.id}
          onValueChange={(user) => {
            if (user) {
              props.onChange(user.id)
              setOpen(false)
              setSearch('')
            }
          }}
        >
          <div ref={anchor} className='w-full sm:w-72'>
            <ComboboxInput
              aria-label={t('Select routing user')}
              placeholder={t('Search users by name or ID...')}
              disabled={props.disabled}
            />
          </div>
          <ComboboxContent anchor={anchor}>
            <ComboboxList>
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
              {!users.isFetching && !users.isError && options.length === 0 && (
                <p className='p-3 text-sm'>{t('No users found')}</p>
              )}
              {options.map((user) => (
                <ComboboxItem key={user.id} value={user}>
                  {user.username} · #{user.id}
                </ComboboxItem>
              ))}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      )}
    </div>
  )
}
