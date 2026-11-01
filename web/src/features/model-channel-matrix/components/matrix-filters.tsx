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
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ChannelStatusFilter } from '@/features/channels/components/channel-status-filter'

import type { MatrixChannel, MatrixSearch } from '../types'

type FilterValues = Pick<MatrixSearch, 'model' | 'channel' | 'group'>

export function MatrixFilters(props: {
  search: MatrixSearch
  groups: string[]
  models: readonly string[]
  channels: readonly Pick<MatrixChannel, 'id' | 'name'>[]
  onSearchChange: (search: MatrixSearch) => void
}) {
  const { t } = useTranslation()
  const { reset, register, handleSubmit, control } = useForm<FilterValues>({
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

  function submitFilters(values: FilterValues) {
    props.onSearchChange({
      ...props.search,
      ...values,
      model_page: 1,
      channel_page: 1,
    })
  }

  return (
    <form
      className='flex shrink-0 flex-wrap items-end gap-3'
      onSubmit={handleSubmit(submitFilters)}
      onKeyDownCapture={(event) => {
        const input = event.target
        if (
          event.key !== 'Enter' ||
          !(input instanceof HTMLInputElement) ||
          (input.id !== 'matrix-model-search' &&
            input.id !== 'matrix-channel-search') ||
          input.getAttribute('aria-activedescendant')
        ) {
          return
        }
        event.preventDefault()
        event.stopPropagation()
        void handleSubmit(submitFilters)()
      }}
    >
      <Field className='min-w-36 flex-1 gap-1.5'>
        <FieldLabel htmlFor='matrix-model-search'>{t('Model')}</FieldLabel>
        <Controller
          name='model'
          control={control}
          render={({ field }) => (
            <Combobox
              id='matrix-model-search'
              placeholder={t('Search models')}
              emptyText={t('No results found')}
              maxLength={255}
              allowCustomValue
              options={props.models.map((model) => ({
                value: model,
                label: model,
              }))}
              value={field.value}
              onValueChange={(value) => field.onChange(value ?? '')}
            />
          )}
        />
      </Field>
      <Field className='min-w-36 flex-1 gap-1.5'>
        <FieldLabel htmlFor='matrix-channel-search'>{t('Channel')}</FieldLabel>
        <Controller
          name='channel'
          control={control}
          render={({ field }) => (
            <Combobox
              id='matrix-channel-search'
              placeholder={t('Channel name or ID')}
              emptyText={t('No results found')}
              maxLength={255}
              allowCustomValue
              options={props.channels.map((channel) => ({
                value: String(channel.id),
                label: `${channel.name} #${channel.id}`,
              }))}
              value={field.value}
              onValueChange={(value) => field.onChange(value ?? '')}
            />
          )}
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
