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
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  deleteRoutingServiceType,
  listRoutingServiceTypes,
  routingPoolKeys,
  saveRoutingServiceType,
  serviceTypeKeys,
  type RoutingServiceType,
} from './api'

const schema = z.object({ name: z.string().trim().min(1).max(128) })

export function ServiceTypeManager(props: {
  value: RoutingServiceType | null
  onSelect: (value: RoutingServiceType | null) => void
  disabled: boolean
  canWrite: boolean
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<RoutingServiceType | null>(null)
  const [removing, setRemoving] = useState<RoutingServiceType | null>(null)
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: '' },
  })
  const types = useQuery({
    queryKey: serviceTypeKeys.all,
    queryFn: listRoutingServiceTypes,
    enabled: open,
    meta: { errorToast: false },
  })
  const save = useMutation({
    mutationFn: (data: z.infer<typeof schema>) => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveRoutingServiceType({
        id: editing?.id ?? 0,
        revision: editing?.revision ?? '',
        name: data.name,
      })
    },
    onSuccess: async (row) => {
      toast.success(t('Saved successfully'))
      setEditing(null)
      form.reset({ name: '' })
      if (props.value?.id === row.id) props.onSelect(row)
      await Promise.all([
        client.invalidateQueries({ queryKey: serviceTypeKeys.all }),
        client.invalidateQueries({ queryKey: ['user-routing-service-types'] }),
        client.invalidateQueries({ queryKey: ['user-routing-pools'] }),
      ])
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })
  const remove = useMutation({
    mutationFn: (row: RoutingServiceType) => {
      if (!props.canWrite) {
        throw new Error(t('No permission to perform this action'))
      }
      return deleteRoutingServiceType(row)
    },
    onSuccess: async (_, row) => {
      setRemoving(null)
      if (props.value?.id === row.id) props.onSelect(null)
      await Promise.all([
        client.invalidateQueries({ queryKey: serviceTypeKeys.all }),
        client.invalidateQueries({ queryKey: routingPoolKeys.all }),
      ])
    },
    onError: (error) => handleServerError(error, t('Failed to delete')),
  })
  const pending = save.isPending || remove.isPending
  return (
    <>
      <Button
        type='button'
        variant='outline'
        disabled={props.disabled}
        onClick={() => setOpen(true)}
      >
        {props.value?.name ?? t('Routing service types')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!pending) setOpen(value)
        }}
        title={t('Routing service types')}
        description={t(
          'Service types share channel pools across users. Scheduling strategies are configured separately.'
        )}
      >
        <div className='space-y-4'>
          <Button
            type='button'
            variant='outline'
            disabled={pending}
            onClick={() => {
              props.onSelect(null)
              setOpen(false)
            }}
          >
            {t('Default routing pool')}
          </Button>
          {types.isPending && <LoadingState />}
          {types.isError && <ErrorState onRetry={() => void types.refetch()} />}
          {types.data?.length === 0 && (
            <EmptyState title={t('No service types')} />
          )}
          <ul className='space-y-2'>
            {types.data?.map((row) => (
              <li
                key={row.id}
                className='flex min-w-0 flex-wrap items-center gap-2 rounded-lg border p-2'
              >
                <Button
                  type='button'
                  variant='ghost'
                  className='min-w-0 flex-1 justify-start break-all whitespace-normal'
                  disabled={pending}
                  onClick={() => {
                    props.onSelect(row)
                    setOpen(false)
                  }}
                >
                  {row.name}
                </Button>
                {props.canWrite && (
                  <>
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={pending}
                      aria-label={t('Edit {{name}}', { name: row.name })}
                      onClick={() => {
                        setEditing(row)
                        form.reset({ name: row.name })
                      }}
                    >
                      {t('Edit')}
                    </Button>
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={pending}
                      aria-label={t('Delete {{name}}', { name: row.name })}
                      onClick={() => setRemoving(row)}
                    >
                      {t('Delete')}
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
          {props.canWrite && (
            <form
              className='space-y-2 border-t pt-4'
              onSubmit={form.handleSubmit((data) => save.mutate(data))}
            >
              <Label htmlFor='routing-service-type-name'>
                {t('Service type name')}
              </Label>
              <Input
                id='routing-service-type-name'
                maxLength={128}
                disabled={pending}
                aria-invalid={!!form.formState.errors.name}
                {...form.register('name')}
              />
              {form.formState.errors.name && (
                <p role='alert' className='text-destructive text-sm'>
                  {t('Please enter a valid name')}
                </p>
              )}
              <div className='flex flex-wrap justify-end gap-2'>
                {editing && (
                  <Button
                    type='button'
                    variant='ghost'
                    disabled={pending}
                    onClick={() => {
                      setEditing(null)
                      form.reset({ name: '' })
                    }}
                  >
                    {t('Cancel')}
                  </Button>
                )}
                <Button type='submit' disabled={pending}>
                  {editing ? t('Save') : t('Create service type')}
                </Button>
              </div>
            </form>
          )}
        </div>
      </Dialog>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(value) => {
          if (!value && !pending) setRemoving(null)
        }}
        title={t('Delete service type')}
        desc={t(
          'Delete service type {{name}}? Assigned service types cannot be deleted.',
          { name: removing?.name }
        )}
        destructive
        isLoading={remove.isPending}
        handleConfirm={() => {
          if (removing) remove.mutate(removing)
        }}
      />
    </>
  )
}
