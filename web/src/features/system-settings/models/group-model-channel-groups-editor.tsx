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
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import { parseGroupModelChannelGroups } from './lib/group-model-channel-groups'

type Props = {
  value: string
  groupOptions: string[]
  onChange: (value: string) => void
  disabled?: boolean
}

export function GroupModelChannelGroupsEditor(props: Props) {
  const { t } = useTranslation()
  const id = useId()
  const draft = useForm({
    resolver: zodResolver(
      z.object({
        userGroup: z.string().min(1, t('Value is required')),
        model: z
          .string()
          .trim()
          .min(1, t('Value is required'))
          .refine((value) => value !== '__proto__', t('Invalid model name')),
      })
    ),
    defaultValues: { userGroup: '', model: '' },
  })
  const policies = parseGroupModelChannelGroups(props.value)
  const groups = props.groupOptions.filter(
    (group) => group !== 'auto' && group !== '__proto__'
  )
  const options = groups.map((group) => ({ label: group, value: group }))
  const rules = Object.entries(policies ?? {}).flatMap(([group, models]) =>
    Object.entries(models).map(([model, pools]) => ({ group, model, pools }))
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Model channel pools')}</CardTitle>
        <CardDescription>
          {t(
            'For each user group and model, combine the channels in the selected pools, then intersect them with the key group channels. Shared channels are allowed even when group names differ.'
          )}
        </CardDescription>
        <CardDescription>
          {t(
            'Unconfigured models keep existing routing. An empty pool selection blocks the model. Changes apply to existing keys after saving.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        {policies === null ? (
          <Alert variant='destructive'>
            <AlertDescription>
              {t(
                'Invalid channel pool policy. Switch to JSON mode to repair it.'
              )}
            </AlertDescription>
          </Alert>
        ) : (
          <>
            <div className='grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-start'>
              <div className='min-w-0 space-y-2'>
                <Label htmlFor={`${id}-group`}>{t('User group')}</Label>
                <NativeSelect
                  id={`${id}-group`}
                  className='w-full'
                  disabled={props.disabled}
                  aria-invalid={!!draft.formState.errors.userGroup}
                  {...draft.register('userGroup')}
                >
                  <NativeSelectOption value=''>
                    {t('Select group')}
                  </NativeSelectOption>
                  {groups.map((group) => (
                    <NativeSelectOption key={group} value={group}>
                      {group}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
                {draft.formState.errors.userGroup && (
                  <p role='alert' className='text-destructive text-sm'>
                    {draft.formState.errors.userGroup.message}
                  </p>
                )}
              </div>
              <div className='min-w-0 space-y-2'>
                <Label htmlFor={`${id}-model`}>{t('Model')}</Label>
                <Input
                  id={`${id}-model`}
                  disabled={props.disabled}
                  aria-invalid={!!draft.formState.errors.model}
                  {...draft.register('model')}
                />
                {draft.formState.errors.model && (
                  <p role='alert' className='text-destructive text-sm'>
                    {draft.formState.errors.model.message}
                  </p>
                )}
              </div>
              <Button
                type='button'
                variant='outline'
                className='sm:mt-6'
                disabled={props.disabled}
                onClick={draft.handleSubmit((values) => {
                  if (!groups.includes(values.userGroup)) {
                    draft.setError('userGroup', { message: t('Select group') })
                    return
                  }
                  if (
                    Object.hasOwn(
                      policies[values.userGroup] ?? {},
                      values.model
                    )
                  ) {
                    draft.setError('model', {
                      message: t(
                        'A channel pool rule already exists for this group and model'
                      ),
                    })
                    return
                  }
                  props.onChange(
                    JSON.stringify(
                      {
                        ...policies,
                        [values.userGroup]: {
                          ...policies[values.userGroup],
                          [values.model]: [],
                        },
                      },
                      null,
                      2
                    )
                  )
                  draft.reset({ userGroup: values.userGroup, model: '' })
                })}
              >
                {t('Add rule')}
              </Button>
            </div>
            {rules.length === 0 && (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>{t('No model channel restrictions')}</EmptyTitle>
                  <EmptyDescription>
                    {t(
                      'Add a rule to choose the channel pools available to a model.'
                    )}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
            {rules.map((rule, index) => (
              <fieldset
                key={JSON.stringify([rule.group, rule.model])}
                aria-label={`${rule.group} / ${rule.model}`}
                disabled={props.disabled}
                className='min-w-0 space-y-2 rounded-lg border p-3'
              >
                <legend className='max-w-full px-1 text-sm break-all'>
                  {rule.group} / {rule.model}
                </legend>
                <div className='flex flex-col gap-3 sm:flex-row sm:items-end'>
                  <div className='min-w-0 flex-1 space-y-2'>
                    <Label htmlFor={`${id}-pools-${index}`}>
                      {t('Channel groups')}
                    </Label>
                    <MultiSelect
                      id={`${id}-pools-${index}`}
                      options={options}
                      selected={rule.pools}
                      disabled={props.disabled}
                      onChange={(pools) => {
                        props.onChange(
                          JSON.stringify(
                            {
                              ...policies,
                              [rule.group]: {
                                ...policies[rule.group],
                                [rule.model]: pools,
                              },
                            },
                            null,
                            2
                          )
                        )
                      }}
                    />
                  </div>
                  <Button
                    type='button'
                    variant='outline'
                    disabled={props.disabled}
                    onClick={() => {
                      const nextModels = { ...policies[rule.group] }
                      delete nextModels[rule.model]
                      const next = { ...policies, [rule.group]: nextModels }
                      if (Object.keys(nextModels).length === 0) {
                        delete next[rule.group]
                      }
                      props.onChange(JSON.stringify(next, null, 2))
                    }}
                  >
                    {t('Remove rule')}
                  </Button>
                </div>
                {rule.pools.length === 0 && (
                  <p className='text-muted-foreground text-sm'>
                    {t('No channel groups selected: this model is blocked.')}
                  </p>
                )}
              </fieldset>
            ))}
          </>
        )}
      </CardContent>
    </Card>
  )
}
