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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getEnabledModels } from '@/features/channels/api'
import { toIntlLocale } from '@/i18n/languages'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  billingGroupModelRatiosSchema,
  groupModelSpecialRatiosSchema,
  type BillingGroupModelRatios,
} from './lib/group-model-special-ratios'

type Rule = { group: string; model: string; ratio: string }
type Props = {
  value: BillingGroupModelRatios
  groupOptions: string[]
  onChange: (value: BillingGroupModelRatios) => void
  disabled?: boolean
}

function removeModelRatio(
  value: BillingGroupModelRatios,
  group: string,
  model: string
): BillingGroupModelRatios {
  const next = { ...value, [group]: { ...value[group] } }
  delete next[group][model]
  if (Object.keys(next[group]).length === 0) delete next[group]
  return next
}

export function GroupModelSpecialRatioEditor(props: Props) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const formID = useId()
  const [draft, setDraft] = useState<Rule | null>(null)
  const [original, setOriginal] = useState<{
    group: string
    model: string
  } | null>(null)
  const modelsQuery = useQuery({
    queryKey: ['enabled-models'],
    queryFn: async () => requireServerSuccess(await getEnabledModels()),
    enabled: draft !== null,
    staleTime: 60_000,
  })
  const rows = Object.entries(props.value).flatMap(([group, models]) =>
    Object.entries(models).map(([model, ratio]) => ({ group, model, ratio }))
  )
  const ratio = Number(draft?.ratio)
  let error = ''
  if (draft) {
    if (!draft.group.trim() || !draft.model.trim() || !draft.ratio.trim()) {
      error = t('Value is required')
    } else if (!Number.isFinite(ratio) || ratio < 0) error = t('Must be ≥ 0')
    else if (
      !billingGroupModelRatiosSchema.safeParse({
        [draft.group]: { [draft.model]: ratio },
      }).success
    ) {
      error = t('Invalid model special ratios')
    } else if (
      Object.hasOwn(props.value[draft.group] ?? {}, draft.model) &&
      (original?.group !== draft.group || original?.model !== draft.model)
    ) {
      error = t('Values must be unique')
    }
  }

  return (
    <div className='min-w-0 space-y-3'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Use the client-facing model ID before channel mapping. Model rules override group rules; unconfigured models keep the existing billing rules.'
        )}
      </p>
      {rows.length === 0 && (
        <EmptyState
          className='min-h-24'
          title={t('No special ratios configured')}
        />
      )}
      {rows.map((row) => (
        <div
          key={JSON.stringify([row.group, row.model])}
          className='grid min-w-0 gap-3 rounded-lg border p-3 sm:grid-cols-[1fr_1fr_auto_auto] sm:items-center'
        >
          <div className='min-w-0 break-all'>
            <span className='text-muted-foreground block text-xs'>
              {t('Billing group')}
            </span>
            {row.group}
          </div>
          <div className='min-w-0 break-all'>
            <span className='text-muted-foreground block text-xs'>
              {t('Public model ID')}
            </span>
            {row.model}
          </div>
          <div>
            <span className='text-muted-foreground block text-xs'>
              {t('Ratio')}
            </span>
            {new Intl.NumberFormat(locale, {
              maximumFractionDigits: 20,
            }).format(row.ratio)}
          </div>
          <div className='flex justify-end gap-1'>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              disabled={props.disabled}
              aria-label={t('Edit model ratio')}
              onClick={() => {
                setOriginal(row)
                setDraft({ ...row, ratio: String(row.ratio) })
              }}
            >
              <Pencil />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              disabled={props.disabled}
              aria-label={t('Remove')}
              onClick={() =>
                props.onChange(
                  removeModelRatio(props.value, row.group, row.model)
                )
              }
            >
              <Trash2 />
            </Button>
          </div>
        </div>
      ))}
      <Button
        type='button'
        variant='outline'
        disabled={props.disabled}
        onClick={() => {
          setOriginal(null)
          setDraft({
            group: props.groupOptions[0] ?? '',
            model: '',
            ratio: '1',
          })
        }}
      >
        <Plus />
        {t('Add model ratio')}
      </Button>
      <Dialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (!open) setDraft(null)
        }}
        title={original ? t('Edit model ratio') : t('Add model ratio')}
        description={t(
          'Use the client-facing model ID before channel mapping. Model rules override group rules; unconfigured models keep the existing billing rules.'
        )}
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setDraft(null)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form={formID}
              disabled={!!error || props.disabled}
            >
              {t('Apply')}
            </Button>
          </>
        }
      >
        {draft && (
          <form
            id={formID}
            className='space-y-4'
            onSubmit={(event) => {
              event.preventDefault()
              event.stopPropagation()
              if (error || props.disabled) return
              let next = props.value
              if (original) {
                next = removeModelRatio(next, original.group, original.model)
              }
              props.onChange({
                ...next,
                [draft.group]: { ...next[draft.group], [draft.model]: ratio },
              })
              setDraft(null)
            }}
          >
            <div className='space-y-2'>
              <Label htmlFor={`${formID}-group`}>{t('Billing group')}</Label>
              <Combobox
                id={`${formID}-group`}
                allowCustomValue
                options={props.groupOptions.map((group) => ({
                  value: group,
                  label: group,
                }))}
                value={draft.group}
                onValueChange={(value) =>
                  setDraft({ ...draft, group: value?.trim() ?? '' })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`${formID}-model`}>{t('Public model ID')}</Label>
              <Combobox
                id={`${formID}-model`}
                allowCustomValue
                options={(modelsQuery.data?.data ?? []).map((model) => ({
                  value: model,
                  label: model,
                }))}
                value={draft.model}
                onValueChange={(value) =>
                  setDraft({ ...draft, model: value?.trim() ?? '' })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`${formID}-ratio`}>{t('Ratio')}</Label>
              <Input
                id={`${formID}-ratio`}
                type='number'
                min={0}
                step='any'
                value={draft.ratio}
                onChange={(event) =>
                  setDraft({ ...draft, ratio: event.target.value })
                }
                aria-invalid={!!error}
              />
            </div>
            {error && (
              <p role='alert' className='text-destructive text-sm'>
                {error}
              </p>
            )}
          </form>
        )}
      </Dialog>
    </div>
  )
}

export function GroupModelSpecialRatioPoliciesEditor(props: {
  value: string
  groupOptions: string[]
  userGroupOptions: string[]
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const [userGroup, setUserGroup] = useState('')
  const selectorID = useId()
  let parsed: unknown
  try {
    parsed = JSON.parse(props.value || '{}')
  } catch {
    parsed = null
  }
  const result = groupModelSpecialRatiosSchema.safeParse(parsed)
  if (!result.success) {
    return (
      <p role='alert' className='text-destructive'>
        {t('Invalid model special ratios')}
      </p>
    )
  }
  const names = [
    ...new Set([...props.userGroupOptions, ...Object.keys(result.data)]),
  ].sort()
  const selected = userGroup || names[0] || ''
  return (
    <div className='space-y-4 rounded-lg border p-4'>
      <h3 className='font-medium'>{t('Model special ratio rules')}</h3>
      <div className='space-y-2'>
        <Label htmlFor={selectorID}>{t('User group')}</Label>
        <Combobox
          id={selectorID}
          allowCustomValue
          options={names.map((name) => ({ value: name, label: name }))}
          value={selected}
          onValueChange={(value) => setUserGroup(value?.trim() ?? '')}
        />
      </div>
      {selected && (
        <GroupModelSpecialRatioEditor
          value={result.data[selected] ?? {}}
          groupOptions={props.groupOptions}
          onChange={(value) => {
            const next = { ...result.data, [selected]: value }
            if (Object.keys(value).length === 0) delete next[selected]
            props.onChange(JSON.stringify(next, null, 2))
          }}
        />
      )}
    </div>
  )
}
