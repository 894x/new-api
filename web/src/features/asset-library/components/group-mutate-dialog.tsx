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
import { Loader2 } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { createAssetGroup, getAssetGroup, updateAssetGroup } from '../api'
import {
  assetGroupFormSchema,
  assetLibraryQueryKeys,
  getAssetLibraryErrorMessage,
  type AssetGroupFormValues,
} from '../lib'
import type { AssetGroup } from '../types'

export function GroupMutateDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  group?: AssetGroup | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isUpdate = !!props.group
  const groupId = props.group?.Id || ''
  const { data: groupDetails } = useQuery({
    queryKey: assetLibraryQueryKeys.group(groupId),
    queryFn: () => getAssetGroup(groupId),
    enabled: props.open && !!groupId,
    staleTime: 0,
    gcTime: 0,
  })
  const form = useForm<AssetGroupFormValues>({
    resolver: zodResolver(assetGroupFormSchema),
    defaultValues: { name: '', description: '' },
  })

  useEffect(() => {
    if (!props.open) return
    const group = groupDetails || props.group
    form.reset({
      name: group?.Name || '',
      description: group?.Description || '',
    })
  }, [form, groupDetails, props.group, props.open])

  const mutation = useMutation({
    mutationFn: async (values: AssetGroupFormValues) => {
      if (props.group) {
        return updateAssetGroup({
          Id: props.group.Id,
          Name: values.name.trim(),
          Description: values.description.trim(),
        })
      }
      return createAssetGroup({
        Name: values.name.trim(),
        Description: values.description.trim() || undefined,
        GroupType: 'AIGC',
      })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.groups(),
      })
      toast.success(
        isUpdate ? t('Asset group updated') : t('Asset group created')
      )
      props.onOpenChange(false)
    },
    onError: (error) => {
      toast.error(
        getAssetLibraryErrorMessage(error, t('Failed to save asset group'))
      )
    },
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={isUpdate ? t('Edit asset group') : t('Create asset group')}
      description={t(
        'Asset groups organize related media and are synchronized to available channels automatically.'
      )}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={mutation.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={form.handleSubmit((values) => mutation.mutate(values))}
            disabled={mutation.isPending}
          >
            {mutation.isPending && <Loader2 className='animate-spin' />}
            {isUpdate ? t('Save') : t('Create asset group')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={t('Asset group name')} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='description'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Description')}</FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    rows={4}
                    placeholder={t('Describe how this asset group is used')}
                  />
                </FormControl>
                <FormDescription>{t('Up to 300 characters.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
