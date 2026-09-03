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
import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Spinner } from '@/components/ui/spinner'
import { getUsers, searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
import { useDebounce } from '@/hooks'
import type { AuthUser } from '@/stores/auth-store'

type AdminAssetLibraryUserScopeProps = {
  currentUser: Pick<AuthUser, 'id' | 'username' | 'display_name'>
  targetUserId?: number
  onTargetUserIdChange: (userId?: number) => void
}

export function AdminAssetLibraryUserScope({
  currentUser,
  targetUserId,
  onTargetUserIdChange,
}: AdminAssetLibraryUserScopeProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [selectedOption, setSelectedOption] = useState<User>()
  const debouncedSearch = useDebounce(search.trim(), 250)
  const usersQuery = useQuery({
    queryKey: ['asset-library', 'user-options', debouncedSearch],
    queryFn: async () => {
      const response = debouncedSearch
        ? await searchUsers({
            keyword: debouncedSearch,
            p: 1,
            page_size: 20,
            sort_by: 'id',
            sort_order: 'asc',
          })
        : await getUsers({
            p: 1,
            page_size: 20,
            sort_by: 'id',
            sort_order: 'asc',
          })
      if (!response.success) {
        throw new Error(response.message || 'Failed to load users')
      }
      return (response.data?.items ?? []).filter((user) => !user.DeletedAt)
    },
    enabled: open,
    staleTime: 30_000,
  })
  const users = (usersQuery.data ?? []).filter(
    (user) => user.id !== currentUser.id
  )
  const selectedUser =
    selectedOption?.id === targetUserId
      ? selectedOption
      : users.find((user) => user.id === targetUserId)
  let selectedLabel = t('My asset library')
  if (targetUserId) {
    selectedLabel = selectedUser
      ? getUserLabel(selectedUser)
      : t('User #{{id}}', { id: targetUserId })
  }

  const selectUser = (user?: User) => {
    setSelectedOption(user)
    onTargetUserIdChange(user?.id)
    setOpen(false)
    setSearch('')
  }

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) setSearch('')
      }}
    >
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            role='combobox'
            aria-label={t('Select asset library owner')}
            aria-expanded={open}
            className='w-full justify-between sm:w-80'
          />
        }
      >
        <span className='truncate'>{selectedLabel}</span>
        <HugeiconsIcon
          icon={ArrowDown01Icon}
          strokeWidth={2}
          className='size-4 shrink-0 opacity-50'
        />
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[var(--anchor-width)] overflow-hidden p-0'
      >
        <Command shouldFilter={false}>
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={t('Search users by name or ID...')}
          />
          <CommandList>
            <CommandGroup>
              <CommandItem
                value='self'
                data-checked={!targetUserId}
                onSelect={() => selectUser()}
              >
                <span className='min-w-0 flex-1'>
                  <span className='block truncate'>
                    {t('My asset library')}
                  </span>
                  <span className='text-muted-foreground block truncate text-xs'>
                    {currentUser.display_name || currentUser.username} · #
                    {currentUser.id}
                  </span>
                </span>
              </CommandItem>
            </CommandGroup>
            <CommandSeparator />
            <CommandGroup heading={t('Users')}>
              {usersQuery.isFetching ? (
                <CommandItem disabled value='loading-users'>
                  <Spinner />
                  {t('Loading users...')}
                </CommandItem>
              ) : null}
              {!usersQuery.isFetching && usersQuery.isError ? (
                <CommandItem disabled value='failed-users'>
                  {t('Failed to load users.')}
                </CommandItem>
              ) : null}
              {!usersQuery.isFetching &&
              !usersQuery.isError &&
              users.length === 0 ? (
                <CommandItem disabled value='no-users'>
                  {t('No users found.')}
                </CommandItem>
              ) : null}
              {!usersQuery.isFetching && !usersQuery.isError
                ? users.map((user) => (
                    <CommandItem
                      key={user.id}
                      value={String(user.id)}
                      data-checked={targetUserId === user.id}
                      onSelect={() => selectUser(user)}
                    >
                      <span className='min-w-0 flex-1'>
                        <span className='block truncate'>
                          {getUserLabel(user)}
                        </span>
                        <span className='text-muted-foreground block truncate text-xs'>
                          #{user.id}
                        </span>
                      </span>
                    </CommandItem>
                  ))
                : null}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function getUserLabel(user: User): string {
  const displayName = user.display_name?.trim()
  if (displayName && displayName !== user.username) {
    return `${displayName} (${user.username})`
  }
  return user.username
}
