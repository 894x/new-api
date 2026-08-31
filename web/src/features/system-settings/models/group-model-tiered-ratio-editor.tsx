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
import { AlertTriangle, Plus, Trash2 } from 'lucide-react'
import {
  memo,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FocusEvent,
  type InputHTMLAttributes,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { getEnabledModels } from '@/features/channels/api'
import { COMMON_TIMEZONES } from '@/features/pricing/lib/billing-expr'
import { formatQuota } from '@/lib/format'

import type {
  GroupModelTieredProgressBasis,
  GroupModelTieredRatioPolicy,
  GroupModelTieredRatios,
} from '../types'
import {
  getUtf8ByteLength,
  getGroupModelTieredValidationMessage,
  getGroupModelTieredPolicyStatus,
  GROUP_NAME_MAX_UTF8_BYTES,
  groupModelTieredRatiosSchema,
  hasDuplicateJsonObjectKey,
  MODEL_NAME_MAX_UTF8_BYTES,
  normalizeGroupModelTieredRatiosShape,
  parseGroupModelTieredRatiosJson,
  serializeGroupModelTieredRatios,
  type GroupModelTieredPolicyStatus,
} from './lib/group-model-tiered-ratio-schema'

type GroupModelTieredRatioEditorProps = {
  value: string
  groupOptions: string[]
  onChange: (value: string) => void
  onValidationChange?: (error: string | null) => void
  now?: number
}

type NumberDraftInputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'type' | 'value' | 'onChange'
> & {
  value: number
  onValueChange: (value: number) => void
  draftErrorKey: string
  invalidDraftMessage: string
  onDraftErrorChange: (draftKey: string, error: string | null) => void
}

type PolicyCardProps = {
  groupName: string
  modelName: string
  policy: GroupModelTieredRatioPolicy
  progressBasis: GroupModelTieredProgressBasis
  config: GroupModelTieredRatios
  groupOptions: string[]
  enabledModels: string[]
  enabledModelsLoaded: boolean
  now: number
  issues: Map<string, string>
  onConfigChange: (config: GroupModelTieredRatios) => void
  draftKey: string
  onDraftErrorChange: (draftKey: string, error: string | null) => void
}

type GroupProgressBasisControlProps = {
  groupName: string
  progressBasis: GroupModelTieredProgressBasis
  onChange: (progressBasis: GroupModelTieredProgressBasis) => void
}

const STATUS_LABELS: Record<GroupModelTieredPolicyStatus, string> = {
  disabled: 'Disabled',
  scheduled: 'Scheduled',
  active: 'Active',
  expired: 'Expired',
}

const STATUS_VARIANTS: Record<
  GroupModelTieredPolicyStatus,
  'secondary' | 'outline' | 'default' | 'destructive'
> = {
  disabled: 'secondary',
  scheduled: 'outline',
  active: 'default',
  expired: 'destructive',
}

function GroupProgressBasisControl(props: GroupProgressBasisControlProps) {
  const { t } = useTranslation()
  const inputId = useId().replaceAll(':', '')
  const label = t('Advance {{group}} tiers by settled quota', {
    group: props.groupName,
  })

  return (
    <div className='bg-muted/30 rounded-lg border p-4'>
      <Field orientation='horizontal'>
        <FieldContent>
          <FieldLabel htmlFor={inputId}>{label}</FieldLabel>
          <FieldDescription>
            {props.progressBasis === 'charged'
              ? t(
                  'Tier progress uses the high-precision settled amount after the current discount; an original price of 20 at 80% advances 16.'
                )
              : t(
                  'Tier progress uses the original model price before any discount.'
                )}{' '}
            {t(
              'This applies to every model in {{group}}, while each model keeps a separate monthly ledger.',
              { group: props.groupName }
            )}
          </FieldDescription>
        </FieldContent>
        <Switch
          id={inputId}
          checked={props.progressBasis === 'charged'}
          onCheckedChange={(checked) =>
            props.onChange(checked ? 'charged' : 'original')
          }
        />
      </Field>
    </div>
  )
}

function formatNumberDraft(value: number): string {
  return Number.isFinite(value) ? String(value) : ''
}

function NumberDraftInput(props: NumberDraftInputProps) {
  const {
    draftErrorKey,
    invalidDraftMessage,
    onDraftErrorChange,
    onValueChange,
    ...inputProps
  } = props
  const [draft, setDraft] = useState(() => formatNumberDraft(props.value))
  const [focused, setFocused] = useState(false)
  const [draftError, setDraftError] = useState('')
  const errorId = `${useId().replaceAll(':', '')}-number-error`

  useEffect(() => {
    if (!focused && !draftError) setDraft(formatNumberDraft(props.value))
  }, [draftError, focused, props.value])

  useEffect(
    () => () => onDraftErrorChange(draftErrorKey, null),
    [draftErrorKey, onDraftErrorChange]
  )

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const nextDraft = event.target.value
    setDraft(nextDraft)
    const parsed = Number(nextDraft)
    if (nextDraft.trim() === '' || !Number.isFinite(parsed)) {
      setDraftError(invalidDraftMessage)
      onDraftErrorChange(draftErrorKey, invalidDraftMessage)
      return
    }
    setDraftError('')
    onDraftErrorChange(draftErrorKey, null)
    onValueChange(parsed)
  }

  const handleBlur = (event: FocusEvent<HTMLInputElement>) => {
    setFocused(false)
    props.onBlur?.(event)
  }

  const describedBy = [inputProps['aria-describedby'], draftError && errorId]
    .filter(Boolean)
    .join(' ')
  const schemaInvalid =
    inputProps['aria-invalid'] === true || inputProps['aria-invalid'] === 'true'

  return (
    <>
      <Input
        {...inputProps}
        type='number'
        value={draft}
        aria-invalid={schemaInvalid || Boolean(draftError)}
        aria-describedby={describedBy || undefined}
        onChange={handleChange}
        onFocus={(event) => {
          setFocused(true)
          props.onFocus?.(event)
        }}
        onBlur={handleBlur}
      />
      {draftError && (
        <p id={errorId} role='alert' className='text-destructive text-sm'>
          {draftError}
        </p>
      )}
    </>
  )
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isRenderSafeConfig(value: unknown): value is GroupModelTieredRatios {
  if (!isObjectRecord(value)) return false

  for (const groupConfig of Object.values(value)) {
    if (
      !isObjectRecord(groupConfig) ||
      (groupConfig.progress_basis !== 'original' &&
        groupConfig.progress_basis !== 'charged') ||
      !isObjectRecord(groupConfig.models)
    ) {
      return false
    }

    for (const policy of Object.values(groupConfig.models)) {
      if (
        !isObjectRecord(policy) ||
        typeof policy.enabled !== 'boolean' ||
        typeof policy.effective_from !== 'number' ||
        (policy.effective_until !== null &&
          typeof policy.effective_until !== 'number') ||
        typeof policy.timezone !== 'string' ||
        !Array.isArray(policy.tiers)
      ) {
        return false
      }

      for (const tier of policy.tiers) {
        if (
          !isObjectRecord(tier) ||
          typeof tier.min_monthly_original_quota !== 'number' ||
          typeof tier.ratio !== 'number'
        ) {
          return false
        }
      }
    }
  }

  return true
}

function parseEditableConfig(value: string): GroupModelTieredRatios | null {
  const validated = parseGroupModelTieredRatiosJson(value)
  if (validated.success) return validated.data

  try {
    const parsed: unknown = JSON.parse(value)
    const normalized = normalizeGroupModelTieredRatiosShape(parsed)
    if (hasDuplicateJsonObjectKey(value) || !isRenderSafeConfig(normalized)) {
      return null
    }
    return normalized
  } catch {
    return null
  }
}

function formatTimestampInTimezone(
  timestamp: number,
  timezone: string
): string {
  if (!Number.isSafeInteger(timestamp) || timestamp < 0) return ''

  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(timestamp * 1000)
    const partMap = Object.fromEntries(
      parts.map((part) => [part.type, part.value])
    )
    return `${partMap.year}-${partMap.month}-${partMap.day}T${partMap.hour}:${partMap.minute}`
  } catch {
    return ''
  }
}

function parseTimestampInTimezone(
  value: string,
  timezone: string
): number | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value.trim())
  if (!match) return null

  const desiredUtc = Date.UTC(
    Number(match[1]),
    Number(match[2]) - 1,
    Number(match[3]),
    Number(match[4]),
    Number(match[5])
  )
  const possibleOffsets = new Set<number>()

  try {
    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
    const sampleRadius = 36 * 60 * 60 * 1000
    const sampleStep = 6 * 60 * 60 * 1000
    for (
      let sample = desiredUtc - sampleRadius;
      sample <= desiredUtc + sampleRadius;
      sample += sampleStep
    ) {
      const parts = formatter.formatToParts(sample)
      const partMap = Object.fromEntries(
        parts.map((part) => [part.type, part.value])
      )
      const representedUtc = Date.UTC(
        Number(partMap.year),
        Number(partMap.month) - 1,
        Number(partMap.day),
        Number(partMap.hour),
        Number(partMap.minute),
        Number(partMap.second)
      )
      possibleOffsets.add(representedUtc - sample)
    }
  } catch {
    return null
  }

  const candidates = new Set<number>()
  for (const offset of possibleOffsets) {
    const timestamp = Math.floor((desiredUtc - offset) / 1000)
    if (formatTimestampInTimezone(timestamp, timezone) === value.trim()) {
      candidates.add(timestamp)
    }
  }

  return candidates.size === 1 ? [...candidates][0] : null
}

function policyPath(groupName: string, modelName: string, ...path: unknown[]) {
  return [groupName, 'models', modelName, ...path].join('.')
}

function PolicyCard(props: PolicyCardProps) {
  const { t } = useTranslation()
  const draftKey = props.draftKey
  const onDraftErrorChange = props.onDraftErrorChange
  const keyDraftErrorKey = `${draftKey}\u0000keys`
  const startDraftErrorKey = `${draftKey}\u0000effective_from`
  const endDraftErrorKey = `${draftKey}\u0000effective_until`
  const inputId = useId().replaceAll(':', '')
  const nextTierId = useRef(props.policy.tiers.length)
  const [tierIds, setTierIds] = useState(() =>
    props.policy.tiers.map((_, index) => `${inputId}-tier-${index}`)
  )
  const [groupDraft, setGroupDraft] = useState(props.groupName)
  const [modelDraft, setModelDraft] = useState(props.modelName)
  const [groupDraftError, setGroupDraftError] = useState('')
  const [modelDraftError, setModelDraftError] = useState('')
  const [startDraft, setStartDraft] = useState(() =>
    formatTimestampInTimezone(
      props.policy.effective_from,
      props.policy.timezone
    )
  )
  const [endDraft, setEndDraft] = useState(() =>
    props.policy.effective_until === null
      ? ''
      : formatTimestampInTimezone(
          props.policy.effective_until,
          props.policy.timezone
        )
  )
  const [startDraftError, setStartDraftError] = useState('')
  const [endDraftError, setEndDraftError] = useState('')

  const groupDraftErrorId = `${inputId}-group-error`
  const modelDraftErrorId = `${inputId}-model-error`

  const reportKeyErrors = (errors: { group: string; model: string }) => {
    setGroupDraftError(errors.group)
    setModelDraftError(errors.model)
    onDraftErrorChange(keyDraftErrorKey, errors.group || errors.model || null)
  }

  useEffect(() => {
    setGroupDraft(props.groupName)
    setModelDraft(props.modelName)
    setGroupDraftError('')
    setModelDraftError('')
    onDraftErrorChange(keyDraftErrorKey, null)
  }, [keyDraftErrorKey, onDraftErrorChange, props.groupName, props.modelName])

  useEffect(() => {
    setStartDraft(
      formatTimestampInTimezone(
        props.policy.effective_from,
        props.policy.timezone
      )
    )
    setStartDraftError('')
    onDraftErrorChange(startDraftErrorKey, null)
  }, [
    onDraftErrorChange,
    props.policy.effective_from,
    props.policy.timezone,
    startDraftErrorKey,
  ])

  useEffect(() => {
    setEndDraft(
      props.policy.effective_until === null
        ? ''
        : formatTimestampInTimezone(
            props.policy.effective_until,
            props.policy.timezone
          )
    )
    setEndDraftError('')
    onDraftErrorChange(endDraftErrorKey, null)
  }, [
    endDraftErrorKey,
    onDraftErrorChange,
    props.policy.effective_until,
    props.policy.timezone,
  ])

  useEffect(
    () => () => {
      onDraftErrorChange(keyDraftErrorKey, null)
      onDraftErrorChange(startDraftErrorKey, null)
      onDraftErrorChange(endDraftErrorKey, null)
    },
    [endDraftErrorKey, keyDraftErrorKey, onDraftErrorChange, startDraftErrorKey]
  )

  useEffect(() => {
    setTierIds((currentIds) => {
      if (currentIds.length === props.policy.tiers.length) return currentIds
      if (currentIds.length > props.policy.tiers.length) {
        return currentIds.slice(0, props.policy.tiers.length)
      }

      const updatedIds = [...currentIds]
      while (updatedIds.length < props.policy.tiers.length) {
        updatedIds.push(`${inputId}-tier-${nextTierId.current}`)
        nextTierId.current += 1
      }
      return updatedIds
    })
  }, [inputId, props.policy.tiers.length])

  const status = getGroupModelTieredPolicyStatus(props.policy, props.now)
  const customModel =
    props.enabledModelsLoaded &&
    modelDraft !== '*' &&
    !props.enabledModels.includes(modelDraft)

  const updatePolicy = (nextPolicy: GroupModelTieredRatioPolicy) => {
    const groupConfig = props.config[props.groupName]
    props.onConfigChange({
      ...props.config,
      [props.groupName]: {
        ...groupConfig,
        models: {
          ...groupConfig.models,
          [props.modelName]: nextPolicy,
        },
      },
    })
  }

  const renamePolicy = (nextGroupName: string, nextModelName: string) => {
    const normalizedGroup = nextGroupName.trim()
    const normalizedModel = nextModelName.trim()
    const nextConfig = { ...props.config }
    const sourceGroup = props.config[props.groupName]
    const oldGroupPolicies = { ...sourceGroup.models }
    delete oldGroupPolicies[props.modelName]
    if (Object.keys(oldGroupPolicies).length === 0) {
      delete nextConfig[props.groupName]
    } else {
      nextConfig[props.groupName] = {
        ...sourceGroup,
        models: oldGroupPolicies,
      }
    }
    const destinationGroup = Object.hasOwn(nextConfig, normalizedGroup)
      ? nextConfig[normalizedGroup]
      : {
          progress_basis: sourceGroup.progress_basis,
          models: {},
        }
    nextConfig[normalizedGroup] = {
      ...destinationGroup,
      models: {
        ...destinationGroup.models,
        [normalizedModel]: props.policy,
      },
    }
    reportKeyErrors({ group: '', model: '' })
    props.onConfigChange(nextConfig)
  }

  const validateKeyDrafts = (nextGroupName: string, nextModelName: string) => {
    const normalizedGroup = nextGroupName.trim()
    const normalizedModel = nextModelName.trim()
    let group = ''
    let model = ''

    if (!normalizedGroup) {
      group = t('Group name is required')
    } else if (normalizedGroup === '__proto__') {
      group = t('Group and model names cannot use __proto__')
    } else if (getUtf8ByteLength(normalizedGroup) > GROUP_NAME_MAX_UTF8_BYTES) {
      group = t('Group name must be at most 128 UTF-8 bytes')
    }

    if (!normalizedModel) {
      model = t('Model name is required')
    } else if (normalizedModel === '__proto__') {
      model = t('Group and model names cannot use __proto__')
    } else if (getUtf8ByteLength(normalizedModel) > MODEL_NAME_MAX_UTF8_BYTES) {
      model = t('Model name must be at most 255 UTF-8 bytes')
    }

    const destinationGroup = Object.hasOwn(props.config, normalizedGroup)
      ? props.config[normalizedGroup]
      : undefined

    if (
      !group &&
      !model &&
      (normalizedGroup !== props.groupName ||
        normalizedModel !== props.modelName) &&
      destinationGroup &&
      Object.hasOwn(destinationGroup.models, normalizedModel)
    ) {
      model = t('Duplicate group and model policies are not allowed')
    }

    return { group, model }
  }

  const commitKeyDrafts = () => {
    const errors = validateKeyDrafts(groupDraft, modelDraft)
    reportKeyErrors(errors)
    if (!errors.group && !errors.model) renamePolicy(groupDraft, modelDraft)
  }

  const removePolicy = () => {
    const nextConfig = { ...props.config }
    const groupConfig = props.config[props.groupName]
    const nextGroupPolicies = { ...groupConfig.models }
    delete nextGroupPolicies[props.modelName]
    if (Object.keys(nextGroupPolicies).length === 0) {
      delete nextConfig[props.groupName]
    } else {
      nextConfig[props.groupName] = {
        ...groupConfig,
        models: nextGroupPolicies,
      }
    }
    props.onConfigChange(nextConfig)
  }

  const updateTier = (
    index: number,
    field: 'min_monthly_original_quota' | 'ratio',
    value: number
  ) => {
    const tiers = props.policy.tiers.map((tier, tierIndex) =>
      tierIndex === index ? { ...tier, [field]: value } : tier
    )
    updatePolicy({ ...props.policy, tiers })
  }

  const addTier = () => {
    const previousTier = props.policy.tiers.at(-1)
    const threshold = previousTier
      ? previousTier.min_monthly_original_quota + 500_000
      : 0
    setTierIds((currentIds) => {
      const id = `${inputId}-tier-${nextTierId.current}`
      nextTierId.current += 1
      return [...currentIds, id]
    })
    updatePolicy({
      ...props.policy,
      tiers: [
        ...props.policy.tiers,
        {
          min_monthly_original_quota: threshold,
          ratio: previousTier?.ratio ?? 1,
        },
      ],
    })
  }

  const removeTier = (index: number) => {
    setTierIds((currentIds) =>
      currentIds.filter((_, tierIndex) => tierIndex !== index)
    )
    updatePolicy({
      ...props.policy,
      tiers: props.policy.tiers.filter((_, tierIndex) => tierIndex !== index),
    })
  }

  const tiersWithIds = props.policy.tiers.map((tier, index) => ({
    id: tierIds[index] ?? `${inputId}-tier-fallback-${index}`,
    tier,
  }))
  const startSchemaError = props.issues.get(
    policyPath(props.groupName, props.modelName, 'effective_from')
  )
  const endSchemaError = props.issues.get(
    policyPath(props.groupName, props.modelName, 'effective_until')
  )
  const timezoneSchemaError = props.issues.get(
    policyPath(props.groupName, props.modelName, 'timezone')
  )
  const startErrorId = `${inputId}-start-error`
  const endErrorId = `${inputId}-end-error`
  const timezoneErrorId = `${inputId}-timezone-error`
  const chargedProgress = props.progressBasis === 'charged'
  const thresholdLabel = chargedProgress
    ? t('Monthly settled quota threshold')
    : t('Monthly original quota threshold')
  const thresholdHelp = chargedProgress
    ? t(
        'Thresholds use monthly settled quota. At 80%, an original price of 20 advances 16; requests crossing a threshold are still split between tiers.'
      )
    : t(
        'Thresholds use monthly original quota. A request crossing a threshold is split between tiers.'
      )

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='flex flex-wrap items-center gap-2'>
          <span>{`${props.groupName} / ${props.modelName}`}</span>
          <Badge variant={STATUS_VARIANTS[status]}>
            {t(STATUS_LABELS[status])}
          </Badge>
        </CardTitle>
        <CardDescription>
          {chargedProgress
            ? t(
                'Monthly tier progress is measured from the settled model price after the active tier discount.'
              )
            : t(
                'Monthly usage is measured from the original model price before any group discount.'
              )}
        </CardDescription>
        <CardAction>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            onClick={removePolicy}
            aria-label={t('Remove tiered discount policy')}
          >
            <Trash2 className='text-destructive h-4 w-4' />
          </Button>
        </CardAction>
      </CardHeader>

      <CardContent className='space-y-5'>
        {status === 'active' && (
          <Alert className='border-warning/40 bg-warning/5'>
            <AlertTriangle className='text-warning' />
            <AlertDescription>
              {t(
                "If this active policy's current period has usage, changing progress basis, tiers, timezone, or end time without a new effective_from can make later settlement fail with a policy hash conflict."
              )}
            </AlertDescription>
          </Alert>
        )}

        <div className='grid gap-4 md:grid-cols-2'>
          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-group`}>{t('Billing group')}</Label>
            <Input
              id={`${inputId}-group`}
              list={`${inputId}-groups`}
              value={groupDraft}
              aria-invalid={groupDraftError !== ''}
              aria-describedby={groupDraftError ? groupDraftErrorId : undefined}
              onChange={(event) => {
                const nextValue = event.target.value
                setGroupDraft(nextValue)
                reportKeyErrors(validateKeyDrafts(nextValue, modelDraft))
              }}
              onBlur={commitKeyDrafts}
              onKeyDown={(event) => {
                if (event.key === 'Enter') event.currentTarget.blur()
                if (event.key === 'Escape') {
                  setGroupDraft(props.groupName)
                  reportKeyErrors(
                    validateKeyDrafts(props.groupName, modelDraft)
                  )
                }
              }}
            />
            <datalist id={`${inputId}-groups`}>
              {props.groupOptions.map((group) => (
                <option key={group} value={group} />
              ))}
            </datalist>
            {groupDraftError && (
              <p
                id={groupDraftErrorId}
                role='alert'
                className='text-destructive text-sm'
              >
                {groupDraftError}
              </p>
            )}
          </div>

          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-model`}>{t('Origin model')}</Label>
            <Input
              id={`${inputId}-model`}
              list={`${inputId}-models`}
              value={modelDraft}
              aria-invalid={modelDraftError !== ''}
              aria-describedby={modelDraftError ? modelDraftErrorId : undefined}
              onChange={(event) => {
                const nextValue = event.target.value
                setModelDraft(nextValue)
                reportKeyErrors(validateKeyDrafts(groupDraft, nextValue))
              }}
              onBlur={commitKeyDrafts}
              onKeyDown={(event) => {
                if (event.key === 'Enter') event.currentTarget.blur()
                if (event.key === 'Escape') {
                  setModelDraft(props.modelName)
                  reportKeyErrors(
                    validateKeyDrafts(groupDraft, props.modelName)
                  )
                }
              }}
            />
            <datalist id={`${inputId}-models`}>
              <option value='*' />
              {props.enabledModels.map((model) => (
                <option key={model} value={model} />
              ))}
            </datalist>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Select an enabled model, enter a custom model, or use * as the fallback.'
              )}
            </p>
            {modelDraftError && (
              <p
                id={modelDraftErrorId}
                role='alert'
                className='text-destructive text-sm'
              >
                {modelDraftError}
              </p>
            )}
          </div>
        </div>

        {customModel && (
          <Alert>
            <AlertTriangle />
            <AlertDescription>
              {t(
                'This model is not currently enabled. The policy will still be saved.'
              )}
            </AlertDescription>
          </Alert>
        )}

        <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-enabled`}>{t('Enabled')}</Label>
            <div className='flex h-9 items-center'>
              <Switch
                id={`${inputId}-enabled`}
                checked={props.policy.enabled}
                onCheckedChange={(enabled) =>
                  updatePolicy({ ...props.policy, enabled })
                }
              />
            </div>
          </div>

          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-start`}>{t('Effective start')}</Label>
            <Input
              id={`${inputId}-start`}
              type='datetime-local'
              value={startDraft}
              aria-invalid={Boolean(startDraftError || startSchemaError)}
              aria-describedby={
                startDraftError || startSchemaError ? startErrorId : undefined
              }
              onChange={(event) => {
                const nextDraft = event.target.value
                setStartDraft(nextDraft)
                if (nextDraft.trim() === '') {
                  const error = t('Effective start is required')
                  setStartDraftError(error)
                  onDraftErrorChange(startDraftErrorKey, error)
                  return
                }

                const timestamp = parseTimestampInTimezone(
                  nextDraft,
                  props.policy.timezone
                )
                if (timestamp === null) {
                  const error = t('Activation time must be a Unix timestamp')
                  setStartDraftError(error)
                  onDraftErrorChange(startDraftErrorKey, error)
                  return
                }

                setStartDraftError('')
                onDraftErrorChange(startDraftErrorKey, null)
                updatePolicy({
                  ...props.policy,
                  effective_from: timestamp,
                })
              }}
            />
            {(startDraftError || startSchemaError) && (
              <p
                id={startErrorId}
                role='alert'
                className='text-destructive text-sm'
              >
                {startDraftError || t(startSchemaError ?? '')}
              </p>
            )}
          </div>

          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-timezone`}>{t('Timezone')}</Label>
            <Input
              id={`${inputId}-timezone`}
              list={`${inputId}-timezones`}
              value={props.policy.timezone}
              aria-invalid={timezoneSchemaError !== undefined}
              aria-describedby={
                timezoneSchemaError ? timezoneErrorId : undefined
              }
              onChange={(event) =>
                updatePolicy({
                  ...props.policy,
                  timezone: event.target.value,
                })
              }
            />
            <datalist id={`${inputId}-timezones`}>
              {COMMON_TIMEZONES.map((timezone) => (
                <option key={timezone.value} value={timezone.value}>
                  {timezone.label}
                </option>
              ))}
            </datalist>
            {timezoneSchemaError && (
              <p
                id={timezoneErrorId}
                role='alert'
                className='text-destructive text-sm'
              >
                {t(timezoneSchemaError)}
              </p>
            )}
          </div>

          <div className='space-y-2'>
            <Label htmlFor={`${inputId}-no-end`}>{t('End time')}</Label>
            <label
              htmlFor={`${inputId}-no-end`}
              className='flex h-9 items-center gap-2 text-sm'
            >
              <input
                id={`${inputId}-no-end`}
                type='checkbox'
                checked={props.policy.effective_until === null}
                onChange={(event) => {
                  if (event.target.checked) {
                    setEndDraftError('')
                    onDraftErrorChange(endDraftErrorKey, null)
                  }
                  updatePolicy({
                    ...props.policy,
                    effective_until: event.target.checked
                      ? null
                      : Math.max(
                          props.policy.effective_from + 86_400,
                          props.now + 86_400
                        ),
                  })
                }}
              />
              {t('No end time')}
            </label>
          </div>
        </div>

        {props.policy.effective_until !== null && (
          <div className='max-w-sm space-y-2'>
            <Label htmlFor={`${inputId}-end`}>{t('Effective end')}</Label>
            <Input
              id={`${inputId}-end`}
              type='datetime-local'
              value={endDraft}
              aria-invalid={Boolean(endDraftError || endSchemaError)}
              aria-describedby={
                endDraftError || endSchemaError ? endErrorId : undefined
              }
              onChange={(event) => {
                const nextDraft = event.target.value
                setEndDraft(nextDraft)
                if (nextDraft.trim() === '') {
                  const error = t('Effective end is required')
                  setEndDraftError(error)
                  onDraftErrorChange(endDraftErrorKey, error)
                  return
                }

                const timestamp = parseTimestampInTimezone(
                  nextDraft,
                  props.policy.timezone
                )
                if (timestamp === null) {
                  const error = t('Activation time must be a Unix timestamp')
                  setEndDraftError(error)
                  onDraftErrorChange(endDraftErrorKey, error)
                  return
                }

                setEndDraftError('')
                onDraftErrorChange(endDraftErrorKey, null)
                updatePolicy({
                  ...props.policy,
                  effective_until: timestamp,
                })
              }}
            />
            {(endDraftError || endSchemaError) && (
              <p
                id={endErrorId}
                role='alert'
                className='text-destructive text-sm'
              >
                {endDraftError || t(endSchemaError ?? '')}
              </p>
            )}
          </div>
        )}

        <div className='space-y-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div>
              <h4 className='font-medium'>{t('Discount tiers')}</h4>
              <p className='text-muted-foreground text-xs'>{thresholdHelp}</p>
            </div>
            <Button type='button' variant='outline' size='sm' onClick={addTier}>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add tier')}
            </Button>
          </div>

          <div className='space-y-2'>
            {tiersWithIds.map((tierEntry, index) => {
              const tier = tierEntry.tier
              const thresholdPath = policyPath(
                props.groupName,
                props.modelName,
                'tiers',
                index,
                'min_monthly_original_quota'
              )
              const ratioPath = policyPath(
                props.groupName,
                props.modelName,
                'tiers',
                index,
                'ratio'
              )
              const thresholdError = props.issues.get(thresholdPath)
              const ratioError = props.issues.get(ratioPath)
              const thresholdErrorId = `${tierEntry.id}-threshold-error`
              const ratioErrorId = `${tierEntry.id}-ratio-error`

              return (
                <div
                  key={tierEntry.id}
                  data-tier-row='true'
                  className='grid gap-3 rounded-lg border p-3 md:grid-cols-[5rem_minmax(0,1fr)_minmax(0,1fr)_2.25rem] md:items-start'
                >
                  <div className='pt-2 text-sm font-medium'>
                    {t('Tier {{number}}', { number: index + 1 })}
                  </div>
                  <div className='space-y-1.5'>
                    <Label htmlFor={`${inputId}-threshold-${index}`}>
                      {thresholdLabel}
                    </Label>
                    <NumberDraftInput
                      id={`${inputId}-threshold-${index}`}
                      aria-label={
                        chargedProgress
                          ? t('Monthly settled quota threshold {{number}}', {
                              number: index + 1,
                            })
                          : t('Monthly original quota threshold {{number}}', {
                              number: index + 1,
                            })
                      }
                      min={0}
                      step={1}
                      disabled={
                        index === 0 && tier.min_monthly_original_quota === 0
                      }
                      value={tier.min_monthly_original_quota}
                      draftErrorKey={`${props.draftKey}\u0000${tierEntry.id}\u0000threshold`}
                      invalidDraftMessage={t('Please enter a valid number')}
                      onDraftErrorChange={props.onDraftErrorChange}
                      aria-invalid={thresholdError !== undefined}
                      aria-describedby={
                        thresholdError ? thresholdErrorId : undefined
                      }
                      onValueChange={(nextValue) =>
                        updateTier(
                          index,
                          'min_monthly_original_quota',
                          nextValue
                        )
                      }
                    />
                    <p className='text-muted-foreground text-xs'>
                      {formatQuota(tier.min_monthly_original_quota)}
                    </p>
                    {thresholdError && (
                      <p
                        id={thresholdErrorId}
                        role='alert'
                        className='text-destructive text-sm'
                      >
                        {t(thresholdError)}
                      </p>
                    )}
                  </div>
                  <div className='space-y-1.5'>
                    <Label htmlFor={`${inputId}-ratio-${index}`}>
                      {t('Discount ratio')}
                    </Label>
                    <NumberDraftInput
                      id={`${inputId}-ratio-${index}`}
                      aria-label={t('Discount ratio {{number}}', {
                        number: index + 1,
                      })}
                      min={0}
                      max={1}
                      step={0.01}
                      value={tier.ratio}
                      draftErrorKey={`${props.draftKey}\u0000${tierEntry.id}\u0000ratio`}
                      invalidDraftMessage={t('Please enter a valid number')}
                      onDraftErrorChange={props.onDraftErrorChange}
                      aria-invalid={ratioError !== undefined}
                      aria-describedby={ratioError ? ratioErrorId : undefined}
                      onValueChange={(nextValue) =>
                        updateTier(index, 'ratio', nextValue)
                      }
                    />
                    <p className='text-muted-foreground text-xs'>
                      {t('{{percent}}% of original price', {
                        percent: Number.isFinite(tier.ratio)
                          ? (tier.ratio * 100).toLocaleString()
                          : 0,
                      })}
                    </p>
                    {ratioError && (
                      <p
                        id={ratioErrorId}
                        role='alert'
                        className='text-destructive text-sm'
                      >
                        {t(ratioError)}
                      </p>
                    )}
                  </div>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    disabled={index === 0 || props.policy.tiers.length === 1}
                    onClick={() => removeTier(index)}
                    aria-label={t('Remove tier {{number}}', {
                      number: index + 1,
                    })}
                  >
                    <Trash2 className='text-destructive h-4 w-4' />
                  </Button>
                </div>
              )
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export const GroupModelTieredRatioEditor = memo(
  function GroupModelTieredRatioEditor(
    props: GroupModelTieredRatioEditorProps
  ) {
    const { t } = useTranslation()
    const onValidationChange = props.onValidationChange
    const draftErrors = useRef(new Map<string, string>())
    const schemaError = useRef<string | null>(null)
    const handleDraftErrorChange = useCallback(
      (draftKey: string, error: string | null) => {
        if (error) {
          draftErrors.current.set(draftKey, error)
        } else {
          draftErrors.current.delete(draftKey)
        }
        onValidationChange?.(
          draftErrors.current.values().next().value ?? schemaError.current
        )
      },
      [onValidationChange]
    )
    const enabledModelsQuery = useQuery({
      queryKey: ['enabled-models'],
      queryFn: getEnabledModels,
    })
    const config = useMemo(
      () => parseEditableConfig(props.value),
      [props.value]
    )
    const validation = useMemo(
      () => (config ? groupModelTieredRatiosSchema.safeParse(config) : null),
      [config]
    )
    const issues = useMemo(() => {
      const issueMap = new Map<string, string>()
      if (validation && !validation.success) {
        for (const issue of validation.error.issues) {
          issueMap.set(
            issue.path.join('.'),
            getGroupModelTieredValidationMessage(issue.message)
          )
        }
      }
      return issueMap
    }, [validation])
    let currentSchemaError: string | null = null
    if (!config) {
      const parsed = parseGroupModelTieredRatiosJson(props.value)
      currentSchemaError = t(
        parsed.success
          ? 'Tiered discount configuration is invalid'
          : parsed.error
      )
    } else if (validation && !validation.success) {
      currentSchemaError = t(
        getGroupModelTieredValidationMessage(
          validation.error.issues[0]?.message
        )
      )
    }
    schemaError.current = currentSchemaError
    useEffect(() => {
      onValidationChange?.(
        draftErrors.current.values().next().value ?? currentSchemaError
      )
    }, [currentSchemaError, onValidationChange])
    const now = props.now ?? Math.floor(Date.now() / 1000)
    const enabledModels = enabledModelsQuery.data?.data ?? []
    const enabledModelsLoaded =
      enabledModelsQuery.data?.success === true && !enabledModelsQuery.isError

    if (!config) {
      return (
        <Alert variant='destructive'>
          <AlertTriangle />
          <AlertDescription>
            {t(
              'Tiered discount JSON cannot be shown visually. Switch to JSON mode to fix it.'
            )}
          </AlertDescription>
        </Alert>
      )
    }

    const changeConfig = (nextConfig: GroupModelTieredRatios) => {
      props.onChange(serializeGroupModelTieredRatios(nextConfig))
    }

    const addPolicy = () => {
      const groupName = props.groupOptions[0] ?? 'default'
      const groupConfig = Object.hasOwn(config, groupName)
        ? config[groupName]
        : {
            progress_basis: 'original' as const,
            models: {},
          }
      let modelName = enabledModels
        .map((model) => model.trim())
        .find(
          (model) =>
            model !== '' &&
            model !== '*' &&
            !Object.hasOwn(groupConfig.models, model)
        )
      if (!modelName) {
        let suffix = 1
        modelName = `custom-model-${suffix}`
        while (Object.hasOwn(groupConfig.models, modelName)) {
          suffix += 1
          modelName = `custom-model-${suffix}`
        }
      }

      changeConfig({
        ...config,
        [groupName]: {
          ...groupConfig,
          models: {
            ...groupConfig.models,
            [modelName]: {
              enabled: false,
              effective_from: now,
              effective_until: null,
              timezone: 'Asia/Shanghai',
              tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
            },
          },
        },
      })
    }

    const groups = Object.entries(config)

    return (
      <section className='space-y-4' aria-labelledby='tiered-discounts-title'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <h3 id='tiered-discounts-title' className='text-base font-semibold'>
              {t('Monthly tiered discounts')}
            </h3>
            <p className='text-muted-foreground mt-1 max-w-3xl text-sm'>
              {t(
                'Configure a billing-group and origin-model policy. Fixed group ratios remain the fallback when no active policy matches.'
              )}
            </p>
          </div>
          <Button type='button' variant='outline' size='sm' onClick={addPolicy}>
            <Plus className='mr-2 h-4 w-4' />
            {t('Add policy')}
          </Button>
        </div>

        {enabledModelsQuery.isError ||
        (enabledModelsQuery.data && !enabledModelsQuery.data.success) ? (
          <Alert>
            <AlertTriangle />
            <AlertDescription>
              {t(
                'Enabled models could not be loaded. You can still enter a model name manually.'
              )}
            </AlertDescription>
          </Alert>
        ) : null}

        {groups.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
            {t(
              'No tiered discount policies. Requests continue to use fixed group ratios.'
            )}
          </div>
        ) : (
          <div className='space-y-4'>
            {groups.map(([groupName, groupConfig]) => (
              <div key={groupName} className='space-y-3'>
                <GroupProgressBasisControl
                  groupName={groupName}
                  progressBasis={groupConfig.progress_basis}
                  onChange={(progressBasis) =>
                    changeConfig({
                      ...config,
                      [groupName]: {
                        ...groupConfig,
                        progress_basis: progressBasis,
                      },
                    })
                  }
                />
                {Object.entries(groupConfig.models).map(
                  ([modelName, policy]) => (
                    <PolicyCard
                      key={`${groupName}\u0000${modelName}`}
                      groupName={groupName}
                      modelName={modelName}
                      policy={policy}
                      progressBasis={groupConfig.progress_basis}
                      config={config}
                      groupOptions={props.groupOptions}
                      enabledModels={enabledModels}
                      enabledModelsLoaded={enabledModelsLoaded}
                      now={now}
                      issues={issues}
                      onConfigChange={changeConfig}
                      draftKey={`${groupName}\u0000${modelName}`}
                      onDraftErrorChange={handleDraftErrorChange}
                    />
                  )
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    )
  }
)
