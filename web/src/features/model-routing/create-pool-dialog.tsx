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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getRoutingCatalog,
  getRoutingPool,
  routingPoolKeys,
  saveRoutingPool,
  type RoutingPool,
} from './api'
import { PoolModeSelect } from './pool-mode-select'

const identitySchema = z.object({
  group: z.string().min(1).max(64),
  model: z.string().min(1).max(255),
  mode: z.enum(['priority', 'smart']),
})
type IdentityValues = z.infer<typeof identitySchema>

export function CreatePoolDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  groups: Record<string, string>
  canWrite: boolean
  onCreated?: (pool: RoutingPool) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<IdentityValues>({
    resolver: zodResolver(identitySchema),
    defaultValues: { group: '', model: '', mode: 'priority' },
  })
  const values = form.watch()
  const catalog = useQuery({
    queryKey: routingPoolKeys.catalog(values.group),
    queryFn: () => getRoutingCatalog(values.group),
    enabled: props.open && !!values.group,
    meta: { errorToast: false },
  })
  const detail = useQuery({
    queryKey: routingPoolKeys.detail(values.group, values.model),
    queryFn: () => getRoutingPool(values.group, values.model),
    enabled: props.open && !!values.group && !!values.model,
    staleTime: 0,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const create = useMutation({
    mutationFn: (data: IdentityValues) => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      if (!detail.data || detail.data.pool) {
        throw new Error(t('This routing pool is already configured.'))
      }
      return saveRoutingPool({
        routing_group: data.group,
        model: data.model,
        mode: data.mode,
        revision: '',
        channels: detail.data.candidates
          .filter((entry) => entry.available)
          .map((entry) => ({
            channel_id: entry.channel_id,
            priority: entry.priority,
            weight: entry.weight,
            enabled: true,
          })),
      })
    },
    onSuccess: async (pool) => {
      toast.success(t('Saved successfully'))
      await queryClient.invalidateQueries({ queryKey: routingPoolKeys.all })
      props.onCreated?.(pool)
      props.onOpenChange(false)
      form.reset()
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!create.isPending) props.onOpenChange(open)
      }}
      title={t('Configure routing pool')}
      description={t(
        'Start from the current channel defaults, then independently configure this model and public group.'
      )}
      footer={
        <Button
          type='button'
          disabled={
            !props.canWrite ||
            create.isPending ||
            !detail.data ||
            detail.isFetching ||
            !!detail.data.pool
          }
          onClick={() =>
            void form.handleSubmit((data) => create.mutate(data))()
          }
        >
          {create.isPending ? t('Saving...') : t('Create')}
        </Button>
      }
    >
      <form
        className='space-y-4'
        onSubmit={(event) => {
          event.preventDefault()
          void form.handleSubmit((data) => create.mutate(data))()
        }}
      >
        <div className='space-y-2'>
          <Label htmlFor='routing-pool-group'>{t('Public group')}</Label>
          <Combobox
            id='routing-pool-group'
            options={Object.entries(props.groups).map(([value, label]) => ({
              value,
              label: `${label || value} (${value})`,
            }))}
            value={values.group || null}
            onValueChange={(value) => {
              form.setValue('group', value ?? '')
              form.setValue('model', '')
            }}
            disabled={create.isPending}
            aria-label={t('Public group')}
            placeholder={t('Select group')}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='routing-pool-model'>{t('Model')}</Label>
          <Combobox
            id='routing-pool-model'
            options={(catalog.data?.models ?? []).map((value) => ({
              value,
              label: value,
            }))}
            value={values.model || null}
            onValueChange={(value) => form.setValue('model', value ?? '')}
            disabled={!values.group || catalog.isPending || create.isPending}
            aria-label={t('Model')}
            placeholder={t('Select model')}
          />
        </div>
        <div className='space-y-2'>
          <Label>{t('Scheduling strategy')}</Label>
          <PoolModeSelect
            value={values.mode}
            onChange={(mode) => form.setValue('mode', mode)}
            disabled={create.isPending}
          />
        </div>
        {detail.data?.pool && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t('This routing pool is already configured.')}
          </p>
        )}
        {(catalog.isError || detail.isError) && (
          <Button
            type='button'
            variant='outline'
            onClick={() => {
              if (catalog.isError) void catalog.refetch()
              if (detail.isError) void detail.refetch()
            }}
          >
            {t('Retry')}
          </Button>
        )}
      </form>
    </Dialog>
  )
}
