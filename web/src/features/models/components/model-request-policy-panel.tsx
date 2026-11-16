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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldTitle } from '@/components/ui/field'
import { ParamOverrideEditorDialog } from '@/features/channels/components/dialogs/param-override-editor-dialog'
import { ParameterCapabilityEditorDialog } from '@/features/channels/components/dialogs/parameter-capability-editor-dialog'
import type { ParameterCapabilityConfig } from '@/features/channels/types'
import {
  createServerError,
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import {
  getModelRequestPolicy,
  saveModelRequestPolicy,
  type ModelRequestPolicy,
} from '../api'
import { modelsQueryKeys } from '../lib/query-keys'

export function ModelRequestPolicyPanel(props: {
  modelName: string
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [...modelsQueryKeys.all, 'request-policy', props.modelName],
    queryFn: async () => {
      const response = requireServerSuccess(
        await getModelRequestPolicy(props.modelName)
      )
      if (!response.data) {
        throw createServerError(response, t('Failed to load model'))
      }
      return response.data
    },
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        description={query.error.message}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return (
    <ModelRequestPolicyEditor
      key={props.modelName}
      policy={query.data}
      modelName={props.modelName}
      onDirtyChange={props.onDirtyChange}
    />
  )
}

function ModelRequestPolicyEditor(props: {
  policy: ModelRequestPolicy
  modelName: string
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [capabilities, setCapabilities] = useState(() =>
    JSON.stringify(props.policy.parameter_capabilities ?? {}, null, 2)
  )
  const [overrides, setOverrides] = useState(() =>
    JSON.stringify(props.policy.param_override ?? {}, null, 2)
  )
  const [saved, setSaved] = useState({ capabilities, overrides })
  const [editor, setEditor] = useState<'capabilities' | 'overrides' | null>(
    null
  )
  const dirty =
    capabilities !== saved.capabilities || overrides !== saved.overrides
  const onDirtyChange = props.onDirtyChange
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange])

  const save = useMutation({
    meta: { errorToast: false },
    mutationFn: async () =>
      requireServerSuccess(
        await saveModelRequestPolicy({
          model: props.modelName,
          parameter_capabilities: JSON.parse(
            capabilities || '{}'
          ) as ParameterCapabilityConfig,
          param_override: JSON.parse(overrides || '{}') as Record<
            string,
            unknown
          >,
        })
      ),
    onSuccess: async () => {
      setSaved({ capabilities, overrides })
      props.onDirtyChange(false)
      await queryClient.invalidateQueries({ queryKey: modelsQueryKeys.all })
      toast.success(t('Saved successfully'))
    },
  })

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4'>
      <Alert>
        <AlertDescription>
          {t(
            'All channels inherit these model defaults. Channel capability fields take precedence. Model overrides run before channel overrides. Rules match the public model name.'
          )}{' '}
          {t(
            'Duplicate override rules across model and channel settings are merged. Different values or conditions remain separate.'
          )}
        </AlertDescription>
      </Alert>
      <FieldGroup>
        <Field>
          <FieldTitle>{t('Parameter capabilities')}</FieldTitle>
          <Button
            variant='outline'
            onClick={() => setEditor('capabilities')}
            disabled={save.isPending}
          >
            {t('Configure capabilities')}
          </Button>
          <pre className='max-h-48 overflow-auto rounded-md border p-3 text-xs break-all whitespace-pre-wrap'>
            {capabilities}
          </pre>
        </Field>
        <Field>
          <FieldTitle>{t('Parameter Override')}</FieldTitle>
          <Button
            variant='outline'
            onClick={() => setEditor('overrides')}
            disabled={save.isPending}
          >
            {t('Configure parameter overrides')}
          </Button>
          <pre className='max-h-48 overflow-auto rounded-md border p-3 text-xs break-all whitespace-pre-wrap'>
            {overrides}
          </pre>
        </Field>
      </FieldGroup>
      {save.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {getServerErrorMessage(save.error, t('Operation failed'))}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap justify-end gap-2'>
        <Button
          variant='outline'
          disabled={save.isPending}
          onClick={() => {
            setCapabilities('{}')
            setOverrides('{}')
          }}
        >
          {t('Clear')}
        </Button>
        <Button
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          {save.isPending ? t('Saving...') : t('Save')}
        </Button>
      </div>
      <ParameterCapabilityEditorDialog
        modelScope
        open={editor === 'capabilities'}
        value={capabilities}
        models={[props.modelName]}
        paramOverrideConfigured={
          Object.keys(JSON.parse(overrides || '{}')).length > 0
        }
        onOpenChange={(open) => {
          if (!open) setEditor(null)
        }}
        onSave={(value) => {
          setCapabilities(value)
          setEditor(null)
        }}
      />
      {editor === 'overrides' && (
        <ParamOverrideEditorDialog
          open
          value={overrides}
          onOpenChange={(open) => {
            if (!open) setEditor(null)
          }}
          onSave={(value) => {
            setOverrides(value)
            setEditor(null)
          }}
        />
      )}
    </div>
  )
}
