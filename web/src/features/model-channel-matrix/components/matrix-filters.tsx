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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ChannelStatusFilter } from '@/features/channels/components/channel-status-filter'

import type { MatrixSearch } from '../types'

type FilterValues = Pick<MatrixSearch, 'model' | 'channel' | 'group'>

export function MatrixFilters(props: {
  search: MatrixSearch
  groups: string[]
  onSearchChange: (search: MatrixSearch) => void
}) {
  const { t } = useTranslation()
  const { reset, register, handleSubmit } = useForm<FilterValues>({
    defaultValues: props.search,
  })
  useEffect(() => {
    reset({
      model: props.search.model,
      channel: props.search.channel,
      group: props.search.group,
    })
  }, [props.search.model, props.search.channel, props.search.group, reset])
  const groups =
    props.groups.includes(props.search.group) || !props.search.group
      ? props.groups
      : [props.search.group, ...props.groups]

  return (
    <form
      className='flex shrink-0 flex-wrap items-end gap-3'
      onSubmit={handleSubmit((values) =>
        props.onSearchChange({
          ...props.search,
          ...values,
          model_page: 1,
          channel_page: 1,
        })
      )}
    >
      <Field className='min-w-36 flex-1 gap-1.5'>
        <FieldLabel htmlFor='matrix-model-search'>{t('Model')}</FieldLabel>
        <Input
          id='matrix-model-search'
          placeholder={t('Search models')}
          maxLength={255}
          {...register('model')}
        />
      </Field>
      <Field className='min-w-36 flex-1 gap-1.5'>
        <FieldLabel htmlFor='matrix-channel-search'>{t('Channel')}</FieldLabel>
        <Input
          id='matrix-channel-search'
          placeholder={t('Channel name or ID')}
          maxLength={255}
          {...register('channel')}
        />
      </Field>
      <Field className='w-auto min-w-32 gap-1.5'>
        <FieldLabel htmlFor='matrix-group'>{t('Group')}</FieldLabel>
        <NativeSelect
          id='matrix-group'
          className='w-full max-w-56'
          {...register('group')}
        >
          <NativeSelectOption value=''>{t('All groups')}</NativeSelectOption>
          {groups.map((group) => (
            <NativeSelectOption key={group} value={group}>
              {group}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </Field>
      <Button type='submit'>{t('Search')}</Button>
      <ChannelStatusFilter
        showAll={props.search.status === 'all'}
        onShowAllChange={(showAll) => {
          props.onSearchChange({
            ...props.search,
            status: showAll ? 'all' : 'enabled',
            model_page: 1,
            channel_page: 1,
          })
        }}
      />
    </form>
  )
}
