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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import { assetLibraryQueryKeys } from '../lib'

export function AssetQuotaDialog(props: {
  path: string
  quotaOverrideMB: number | null
  defaultQuotaMB: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = z.object({
    quotaMB: z
      .string()
      .trim()
      .refine(
        (value) =>
          value === '' ||
          (Number.isInteger(Number(value)) &&
            Number(value) >= 0 &&
            Number(value) <= 1_000_000),
        t('Asset quota must be an integer between 0 and 1000000 MB')
      ),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      quotaMB:
        props.quotaOverrideMB === null ? '' : String(props.quotaOverrideMB),
    },
  })
  const mutation = useMutation({
    mutationFn: async (quotaMB: number | null) => {
      const response = await api.put<{ success: boolean; message?: string }>(
        props.path,
        { quota_mb: quotaMB }
      )
      if (!response.data.success) {
        throw new Error(response.data.message || 'Failed to update asset quota')
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.all,
      })
      toast.success(t('Asset quota updated.'))
      props.onClose()
    },
    onError: () =>
      form.setError('root', { message: t('Failed to update asset quota') }),
  })
  const submit = form.handleSubmit((values) => {
    form.clearErrors('root')
    mutation.mutate(values.quotaMB === '' ? null : Number(values.quotaMB))
  })

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
    >
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Set user quota')}</DialogTitle>
          <DialogDescription>
            {t(
              'Leave blank to inherit the global default ({{quota}} MB). Set 0 to prevent new storage usage.',
              { quota: props.defaultQuotaMB }
            )}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={submit} className='flex flex-col gap-4'>
            <FieldGroup>
              <FormField
                control={form.control}
                name='quotaMB'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('User asset quota (MB)')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        max={1_000_000}
                        step={1}
                        placeholder={String(props.defaultQuotaMB)}
                        disabled={mutation.isPending}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </FieldGroup>
            {form.formState.errors.root ? (
              <p role='alert' className='text-destructive text-sm'>
                {form.formState.errors.root.message}
              </p>
            ) : null}
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={props.onClose}
                disabled={mutation.isPending}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                disabled={mutation.isPending || !form.formState.isDirty}
              >
                {t('Save Changes')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
