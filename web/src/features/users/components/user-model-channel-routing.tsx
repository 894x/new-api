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
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Label } from '@/components/ui/label'

import { getUserChannelRouting } from '../api'
import { UserChannelRoutingRuleEditor } from './user-channel-routing-dialog'

export function UserModelChannelRouting(props: {
  userId: number
  poolModels: string[]
  readOnly?: boolean
  onDirtyChange: (model: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const [modelToAdd, setModelToAdd] = useState<string | null>(null)
  const [addedModels, setAddedModels] = useState<string[]>([])
  const catalog = useQuery({
    queryKey: ['user-channel-routing-models', props.userId],
    queryFn: () => getUserChannelRouting(props.userId),
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  if (catalog.isPending) return <LoadingState />
  if (catalog.isError && !catalog.data?.data) {
    return <ErrorState onRetry={() => void catalog.refetch()} />
  }
  const data = catalog.data.data
  const keyGroups = Object.keys(data?.key_groups ?? {}).sort()
  if (keyGroups.length === 0) {
    return <EmptyState title={t('No public Key groups available')} />
  }
  const models = [
    ...new Set([
      ...(data?.configured_models ?? []),
      ...props.poolModels,
      ...addedModels,
    ]),
  ]
  return (
    <div className='flex flex-col gap-4'>
      {catalog.isError && (
        <Alert variant='destructive'>
          <AlertDescription className='flex items-center justify-between gap-3'>
            {t('Failed to load models')}
            <Button
              type='button'
              variant='outline'
              onClick={() => void catalog.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {models.map((model) => (
        <section
          key={model}
          aria-label={model}
          className='flex min-w-0 flex-col gap-3 rounded-lg border p-3'
        >
          <div>
            <p className='text-muted-foreground text-xs'>{t('Model')}</p>
            <h4 className='mt-1 font-medium break-all'>{model}</h4>
          </div>
          <UserChannelRoutingRuleEditor
            userId={props.userId}
            model={model}
            keyGroups={keyGroups}
            configuredGroups={data?.configured_key_groups?.[model] ?? []}
            readOnly={props.readOnly}
            onDirtyChange={(dirty) => props.onDirtyChange(model, dirty)}
          />
        </section>
      ))}
      <div className='flex flex-col gap-2'>
        <Label htmlFor='user-routing-model'>{t('Model')}</Label>
        <div className='flex flex-wrap gap-2'>
          <Combobox
            id='user-routing-model'
            aria-label={t('Model')}
            className='min-w-0 flex-1'
            options={(data?.models ?? [])
              .filter((model) => !models.includes(model))
              .map((model) => ({ value: model, label: model }))}
            value={modelToAdd}
            onValueChange={setModelToAdd}
            allowCustomValue
            disabled={props.readOnly}
            placeholder={t('Select model')}
          />
          <Button
            type='button'
            variant='outline'
            disabled={
              props.readOnly ||
              !modelToAdd?.trim() ||
              models.includes(modelToAdd.trim())
            }
            onClick={() => {
              const model = modelToAdd?.trim()
              if (props.readOnly || !model || models.includes(model)) return
              setAddedModels((current) => [...current, model])
              setModelToAdd(null)
            }}
          >
            <Plus /> {t('Add model')}
          </Button>
        </div>
      </div>
    </div>
  )
}
