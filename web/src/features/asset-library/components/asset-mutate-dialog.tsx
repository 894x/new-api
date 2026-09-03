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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { createAsset, listAllAssetGroups, updateAsset } from '../api'
import {
  assetFormSchema,
  assetLibraryQueryKeys,
  assetUpdateFormSchema,
  getAssetLibraryErrorMessage,
  type AssetFormValues,
  type AssetUpdateFormValues,
} from '../lib'
import type { Asset } from '../types'

type AssetMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  asset?: Asset | null
}

const ASSET_TYPE_OPTIONS = ['Image', 'Video', 'Audio'] as const

export function AssetMutateDialog(props: AssetMutateDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isUpdate = !!props.asset
  const { data: groups = [], isLoading: groupsLoading } = useQuery({
    queryKey: assetLibraryQueryKeys.groupOptions(),
    queryFn: () => listAllAssetGroups(),
    enabled: props.open && !isUpdate,
  })
  const createForm = useForm<AssetFormValues>({
    resolver: zodResolver(assetFormSchema),
    defaultValues: { name: '', groupId: '', assetType: 'Image', url: '' },
  })
  const updateForm = useForm<AssetUpdateFormValues>({
    resolver: zodResolver(assetUpdateFormSchema),
    defaultValues: { name: '' },
  })

  useEffect(() => {
    if (!props.open) return
    if (props.asset) {
      updateForm.reset({ name: props.asset.Name || '' })
      return
    }
    createForm.reset({
      name: '',
      groupId: groups.length === 1 ? groups[0].Id : '',
      assetType: 'Image',
      url: '',
    })
  }, [createForm, groups, props.asset, props.open, updateForm])

  const mutation = useMutation({
    mutationFn: async (values: AssetFormValues | AssetUpdateFormValues) => {
      if (props.asset) {
        return updateAsset({
          Id: props.asset.Id,
          Name: values.name.trim(),
        })
      }
      const createValues = values as AssetFormValues
      return createAsset({
        GroupId: createValues.groupId,
        URL: createValues.url.trim(),
        AssetType: createValues.assetType,
        Name: createValues.name.trim() || undefined,
      })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: assetLibraryQueryKeys.assets(),
      })
      toast.success(isUpdate ? t('Asset updated') : t('Asset added'))
      props.onOpenChange(false)
    },
    onError: (error) => {
      toast.error(getAssetLibraryErrorMessage(error, t('Failed to save asset')))
    },
  })

  if (isUpdate) {
    return (
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Edit asset')}
        description={t(
          'The source URL and media type cannot be changed after creation.'
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
              onClick={updateForm.handleSubmit((values) =>
                mutation.mutate(values)
              )}
              disabled={mutation.isPending}
            >
              {mutation.isPending && <Loader2 className='animate-spin' />}
              {t('Save')}
            </Button>
          </>
        }
      >
        <Form {...updateForm}>
          <form
            onSubmit={updateForm.handleSubmit((values) =>
              mutation.mutate(values)
            )}
            className='space-y-4'
          >
            <FormField
              control={updateForm.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Optional asset name')} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
      </Dialog>
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Add asset')}
      description={t(
        'Provide a publicly accessible URL. Local file upload is not supported.'
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
            onClick={createForm.handleSubmit((values) =>
              mutation.mutate(values)
            )}
            disabled={
              mutation.isPending || groupsLoading || groups.length === 0
            }
          >
            {mutation.isPending && <Loader2 className='animate-spin' />}
            {t('Add asset')}
          </Button>
        </>
      }
    >
      <Form {...createForm}>
        <form
          onSubmit={createForm.handleSubmit((values) =>
            mutation.mutate(values)
          )}
          className='space-y-4'
        >
          <FormField
            control={createForm.control}
            name='groupId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Asset Group')}</FormLabel>
                <Select
                  items={groups.map((group) => ({
                    label: group.Name,
                    value: group.Id,
                  }))}
                  value={field.value}
                  onValueChange={(value) => field.onChange(value ?? '')}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select an asset group')} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {groups.map((group) => (
                        <SelectItem key={group.Id} value={group.Id}>
                          {group.Name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                {groups.length === 0 && !groupsLoading && (
                  <FormDescription>
                    {t('Create an asset group before adding assets.')}
                  </FormDescription>
                )}
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={createForm.control}
            name='assetType'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Type')}</FormLabel>
                <Select
                  items={ASSET_TYPE_OPTIONS.map((value) => ({
                    label: t(value),
                    value,
                  }))}
                  value={field.value}
                  onValueChange={(value) => value && field.onChange(value)}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {ASSET_TYPE_OPTIONS.map((value) => (
                        <SelectItem key={value} value={value}>
                          {t(value)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={createForm.control}
            name='url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Public URL')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    inputMode='url'
                    placeholder='https://example.com/asset.png'
                  />
                </FormControl>
                <FormDescription>
                  {t('The upstream service must be able to download this URL.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={createForm.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={t('Optional asset name')} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
