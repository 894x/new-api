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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { updateModelDocStatus } from '../api'
import { modelsQueryKeys } from '../lib/query-keys'
import type { Model } from '../types'

type ModelDocSwitchProps = {
  model: Model
}

export function ModelDocSwitch(props: ModelDocSwitchProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: (enabled: boolean) =>
      updateModelDocStatus(props.model.id, enabled ? 1 : 0),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to update model document'))
        return
      }
      toast.success(t('Model document updated'))
      queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
      queryClient.invalidateQueries({
        queryKey: modelsQueryKeys.detail(props.model.id),
      })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to update model document'))
    },
  })

  const control = (
    <Switch
      checked={props.model.doc_enabled === 1}
      onCheckedChange={(checked) => mutation.mutate(checked)}
      disabled={!props.model.doc_available || mutation.isPending}
      aria-label={t('Enable model document')}
    />
  )

  if (props.model.doc_available) return control

  return (
    <Tooltip>
      <TooltipTrigger render={<span className='inline-flex' />}>
        {control}
      </TooltipTrigger>
      <TooltipContent>
        {t('No HTML document is available for this model.')}
      </TooltipContent>
    </Tooltip>
  )
}
