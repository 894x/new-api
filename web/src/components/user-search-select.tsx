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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'

import { searchUsers } from '@/features/users/api'

type UserSearchSelectProps = {
  /** 逗号分隔的已选用户名 */
  value: string
  /** 返回逗号分隔的用户名 */
  onChange: (value: string) => void
  disabled?: boolean
}

/**
 * 用户模糊搜索多选器：输入关键字异步搜索平台用户，点击选中生成 chip；
 * 支持手动输入任意用户名（allowCreate），值为逗号分隔字符串。
 */
export function UserSearchSelect(props: UserSearchSelectProps) {
  const { t } = useTranslation()
  const inputId = useId()

  const searchQuery = useQuery({
    queryKey: ['user-search-select', ''],
    queryFn: () =>
      searchUsers({ keyword: '', p: 1, page_size: 50, sort_by: 'id' }),
    staleTime: 60_000,
  })

  const options = (searchQuery.data?.data?.items ?? []).map((user) => ({
    label: user.username,
    value: user.username,
    hint: user.display_name || undefined,
  }))

  const selected = props.value
    .split(/[,，\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)

  const handleChange = (values: string[]) => {
    props.onChange(values.join(', '))
  }

  return (
    <MultiSelect
      id={inputId}
      options={options}
      selected={selected}
      onChange={handleChange}
      placeholder={t('Search and select users to monitor')}
      emptyText={t('No users found')}
      allowCreate
      createLabel={t('Add "{{value}}"')}
      disabled={props.disabled}
      className='w-full'
    />
  )
}
