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
import {
  CircleAlert,
  FlaskConical,
  Plus,
  Search,
  Settings2,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import {
  evaluateParameterCapabilities,
  isBillingSensitiveParameter,
  PARAMETER_CAPABILITY_CATALOG,
  parseParameterCapabilityConfig,
  resolveParameterCapabilities,
  stringifyParameterCapabilityConfig,
  validateParameterCapabilityConfig,
  type CapabilityEvaluation,
  type ParameterCapabilityConfigError,
} from '../../lib/parameter-capabilities'
import type {
  ModelParameterCapabilityRule,
  ParameterCapability,
  ParameterCapabilityAction,
  ParameterCapabilityConfig,
} from '../../types'

export interface ParameterCapabilityEditorDialogProps {
  open: boolean
  value: string
  models: string[]
  paramOverrideConfigured: boolean
  onOpenChange: (open: boolean) => void
  onSave: (value: string) => void
}

type ScopeSelection = { type: 'default' } | { type: 'rule'; index: number }

const SUPPORT_OPTIONS = [
  { value: 'inherit', label: 'Inherit' },
  { value: 'supported', label: 'Supported' },
  { value: 'unsupported', label: 'Unsupported' },
] as const

const ACTION_OPTIONS: Array<{
  value: ParameterCapabilityAction
  label: string
}> = [
  { value: 'reject', label: 'Reject request' },
  { value: 'drop', label: 'Drop parameter' },
  { value: 'clamp', label: 'Clamp to range' },
]

export function ParameterCapabilityEditorDialog(
  props: ParameterCapabilityEditorDialogProps
) {
  if (!props.open) return null
  return (
    <ParameterCapabilityEditorSession
      key={props.value}
      value={props.value}
      models={props.models}
      paramOverrideConfigured={props.paramOverrideConfigured}
      onOpenChange={props.onOpenChange}
      onSave={props.onSave}
    />
  )
}

function ParameterCapabilityEditorSession(
  props: Omit<ParameterCapabilityEditorDialogProps, 'open'>
) {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ParameterCapabilityConfig>(() =>
    parseParameterCapabilityConfig(props.value)
  )
  const [selection, setSelection] = useState<ScopeSelection>({
    type: 'default',
  })
  const [parameterSearch, setParameterSearch] = useState('')
  const [customParameter, setCustomParameter] = useState('')

  const errors = validateParameterCapabilityConfig(config)
  const errorKeyCounts = new Map<string, number>()
  const rules = config.rules || []
  const selectedRule =
    selection.type === 'rule' ? rules[selection.index] : undefined
  const selectedParameters =
    selection.type === 'default'
      ? config.defaults || {}
      : selectedRule?.parameters || {}
  const previewModel =
    selectedRule?.selector.value || props.models[0] || 'model-name'
  const resolved = resolveParameterCapabilities(config, previewModel)

  const catalogPaths = PARAMETER_CAPABILITY_CATALOG.map((item) => item.path)
  const parameterPaths = [
    ...new Set([...catalogPaths, ...Object.keys(selectedParameters)]),
  ]
  const parameterQuery = parameterSearch.trim().toLowerCase()
  const visibleParameters = parameterQuery
    ? parameterPaths.filter((path) =>
        path.toLowerCase().includes(parameterQuery)
      )
    : parameterPaths

  function updateSelectedParameters(
    nextParameters: Record<string, ParameterCapability>
  ): void {
    setConfig((current) => {
      if (selection.type === 'default') {
        return { ...current, defaults: nextParameters }
      }
      const nextRules = [...(current.rules || [])]
      const rule = nextRules[selection.index]
      if (!rule) return current
      nextRules[selection.index] = { ...rule, parameters: nextParameters }
      return { ...current, rules: nextRules }
    })
  }

  function updateCapability(
    path: string,
    capability: ParameterCapability | null
  ): void {
    const nextParameters = { ...selectedParameters }
    if (capability) nextParameters[path] = capability
    else delete nextParameters[path]
    updateSelectedParameters(nextParameters)
  }

  function updateSelectedRule(
    patch: Partial<ModelParameterCapabilityRule>
  ): void {
    if (selection.type !== 'rule') return
    setConfig((current) => {
      const nextRules = [...(current.rules || [])]
      const rule = nextRules[selection.index]
      if (!rule) return current
      nextRules[selection.index] = { ...rule, ...patch }
      return { ...current, rules: nextRules }
    })
  }

  function addRule(type: 'pattern' | 'exact'): void {
    const existingExactModels = new Set(
      rules
        .filter((rule) => rule.selector.type === 'exact')
        .map((rule) => rule.selector.value)
    )
    const exactModel =
      props.models.find((model) => !existingExactModels.has(model)) ||
      props.models[0] ||
      ''
    const nextRule: ModelParameterCapabilityRule = {
      name: '',
      selector: {
        type,
        value: type === 'exact' ? exactModel : '*',
      },
      parameters: {},
    }
    const nextRules = [...rules, nextRule]
    setConfig((current) => ({ ...current, rules: nextRules }))
    setSelection({ type: 'rule', index: nextRules.length - 1 })
  }

  function deleteSelectedRule(): void {
    if (selection.type !== 'rule') return
    setConfig((current) => ({
      ...current,
      rules: (current.rules || []).filter(
        (_, index) => index !== selection.index
      ),
    }))
    setSelection({ type: 'default' })
  }

  function handleSave(): void {
    if (errors.length > 0) {
      toast.error(t('Fix parameter capability errors before saving.'))
      return
    }
    props.onSave(stringifyParameterCapabilityConfig(config))
    props.onOpenChange(false)
  }

  function addCustomParameter(): void {
    const path = customParameter.trim()
    if (!path || selectedParameters[path]) return
    updateCapability(path, { on_violation: 'reject' })
    setCustomParameter('')
  }

  return (
    <Dialog
      open
      onOpenChange={props.onOpenChange}
      title={t('Model Parameter Capabilities')}
      description={t(
        'Configure channel defaults, model family rules, and exact model overrides.'
      )}
      contentClassName='flex max-h-[94vh] flex-col gap-0 p-0 sm:max-w-7xl'
      headerClassName='border-b px-6 py-4'
      footerClassName='border-t px-6 py-4'
      bodyClassName='h-full p-0'
      contentHeight='min(78vh, 820px)'
      footer={
        <>
          <div className='mr-auto flex items-center gap-2 text-sm'>
            <Badge variant={errors.length > 0 ? 'destructive' : 'secondary'}>
              {errors.length > 0
                ? t('{{count}} configuration error(s)', {
                    count: errors.length,
                  })
                : t('Configuration valid')}
            </Badge>
          </div>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSave}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <Tabs defaultValue='capabilities' className='h-full gap-0'>
        <div className='flex items-center justify-between border-b px-4 py-2'>
          <TabsList>
            <TabsTrigger value='capabilities'>
              <Settings2 data-icon='inline-start' />
              {t('Capabilities')}
            </TabsTrigger>
            <TabsTrigger value='validation'>
              <FlaskConical data-icon='inline-start' />
              {t('Request validation')}
            </TabsTrigger>
          </TabsList>
          <span className='text-muted-foreground text-xs'>
            {t('Exact model overrides take precedence over pattern rules.')}
          </span>
        </div>

        {props.paramOverrideConfigured && (
          <Alert className='mx-4 mt-4'>
            <CircleAlert aria-hidden='true' />
            <AlertTitle>{t('Parameter Override')}</AlertTitle>
            <AlertDescription>
              {t(
                'Parameter override runs first; model parameter capabilities then validate the final upstream request.'
              )}
            </AlertDescription>
          </Alert>
        )}

        <TabsContent value='capabilities' className='min-h-0 overflow-hidden'>
          <div className='grid h-full min-h-0 grid-cols-[260px_minmax(0,1fr)]'>
            <ScopeSidebar
              rules={rules}
              selection={selection}
              onSelect={setSelection}
              onAddRule={addRule}
            />

            <div className='flex min-h-0 min-w-0 flex-col'>
              <div className='flex flex-col gap-3 border-b px-5 py-4'>
                {selection.type === 'default' ? (
                  <div>
                    <h3 className='font-medium'>{t('Channel default')}</h3>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'These constraints apply to every model unless a matching rule overrides them.'
                      )}
                    </p>
                  </div>
                ) : (
                  <RuleEditorHeader
                    rule={selectedRule}
                    models={props.models}
                    onChange={updateSelectedRule}
                    onDelete={deleteSelectedRule}
                  />
                )}

                <div className='flex items-center gap-3 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('Effective preview for')}
                  </span>
                  <Badge variant='outline'>{previewModel}</Badge>
                  <span className='text-muted-foreground'>
                    {t('{{count}} effective parameter(s)', {
                      count: Object.keys(resolved).length,
                    })}
                  </span>
                </div>
              </div>

              {errors.length > 0 && (
                <Alert variant='destructive' className='m-4 mb-0'>
                  <CircleAlert />
                  <AlertTitle>{t('Configuration needs attention')}</AlertTitle>
                  <AlertDescription>
                    {errors.map((error) => {
                      const baseKey = `${error.code}:${error.scope}:${error.path}`
                      const occurrence = (errorKeyCounts.get(baseKey) || 0) + 1
                      errorKeyCounts.set(baseKey, occurrence)
                      return (
                        <div key={`${baseKey}:${occurrence}`}>
                          <CapabilityConfigErrorMessage error={error} />
                        </div>
                      )
                    })}
                  </AlertDescription>
                </Alert>
              )}

              <div className='flex items-center gap-3 px-5 py-4'>
                <InputGroup className='max-w-md'>
                  <InputGroupAddon>
                    <Search aria-hidden='true' />
                  </InputGroupAddon>
                  <InputGroupInput
                    value={parameterSearch}
                    onChange={(event) => setParameterSearch(event.target.value)}
                    placeholder={t('Search parameters')}
                    aria-label={t('Search parameters')}
                  />
                </InputGroup>
                <div className='ml-auto flex items-center gap-2'>
                  <Input
                    value={customParameter}
                    onChange={(event) => setCustomParameter(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        addCustomParameter()
                      }
                    }}
                    placeholder={t('Custom parameter path')}
                    aria-label={t('Custom parameter path')}
                    className='w-52'
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={addCustomParameter}
                    disabled={!customParameter.trim()}
                  >
                    <Plus data-icon='inline-start' />
                    {t('Add')}
                  </Button>
                </div>
              </div>

              <ScrollArea className='min-h-0 flex-1 px-5 pb-5'>
                <div className='flex flex-col gap-3'>
                  {visibleParameters.map((path) => (
                    <ParameterCapabilityRow
                      key={path}
                      path={path}
                      configured={selectedParameters[path]}
                      effective={resolved[path]}
                      onChange={(capability) =>
                        updateCapability(path, capability)
                      }
                    />
                  ))}
                </div>
              </ScrollArea>
            </div>
          </div>
        </TabsContent>

        <TabsContent value='validation' className='min-h-0 overflow-hidden'>
          <CapabilityValidationPanel config={config} models={props.models} />
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}

function ScopeSidebar(props: {
  rules: ModelParameterCapabilityRule[]
  selection: ScopeSelection
  onSelect: (selection: ScopeSelection) => void
  onAddRule: (type: 'pattern' | 'exact') => void
}) {
  const { t } = useTranslation()
  const ruleKeyCounts = new Map<string, number>()
  return (
    <aside className='bg-muted/20 flex min-h-0 flex-col border-r'>
      <div className='flex flex-col gap-2 border-b p-3'>
        <Button
          type='button'
          variant={props.selection.type === 'default' ? 'secondary' : 'ghost'}
          className='justify-start'
          onClick={() => props.onSelect({ type: 'default' })}
        >
          <Settings2 data-icon='inline-start' />
          {t('Channel default')}
        </Button>
      </div>
      <ScrollArea className='min-h-0 flex-1 p-3'>
        <div className='flex flex-col gap-2'>
          <div className='flex items-center justify-between px-2'>
            <span className='text-muted-foreground text-xs font-medium'>
              {t('Model rules')}
            </span>
            <Badge variant='secondary'>{props.rules.length}</Badge>
          </div>
          {props.rules.map((rule, index) => {
            const baseKey = `${rule.selector.type}:${rule.selector.value}:${rule.name || 'unnamed'}`
            const occurrence = (ruleKeyCounts.get(baseKey) || 0) + 1
            ruleKeyCounts.set(baseKey, occurrence)
            return (
              <Button
                key={`${baseKey}:${occurrence}`}
                type='button'
                variant={
                  props.selection.type === 'rule' &&
                  props.selection.index === index
                    ? 'secondary'
                    : 'ghost'
                }
                className='h-auto min-h-9 justify-start py-2 text-left'
                onClick={() => props.onSelect({ type: 'rule', index })}
              >
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-sm'>
                    {rule.name || rule.selector.value || t('Untitled rule')}
                  </div>
                  <div className='text-muted-foreground truncate text-xs'>
                    {rule.selector.type === 'exact'
                      ? t('Exact model')
                      : t('Model pattern')}{' '}
                    · {Object.keys(rule.parameters || {}).length}
                  </div>
                </div>
              </Button>
            )
          })}
        </div>
      </ScrollArea>
      <Separator />
      <div className='grid grid-cols-2 gap-2 p-3'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => props.onAddRule('pattern')}
        >
          <Plus data-icon='inline-start' />
          {t('Pattern')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => props.onAddRule('exact')}
        >
          <Plus data-icon='inline-start' />
          {t('Model')}
        </Button>
      </div>
    </aside>
  )
}

function RuleEditorHeader(props: {
  rule?: ModelParameterCapabilityRule
  models: string[]
  onChange: (patch: Partial<ModelParameterCapabilityRule>) => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  if (!props.rule) return null
  const selectorTypeItems = [
    { value: 'pattern', label: t('Model pattern') },
    { value: 'exact', label: t('Exact model') },
  ]
  return (
    <FieldGroup className='gap-3'>
      <div className='grid grid-cols-[minmax(0,1fr)_150px_minmax(220px,1fr)_auto] items-end gap-3'>
        <Field>
          <FieldLabel htmlFor='capability-rule-name'>
            {t('Rule name')}
          </FieldLabel>
          <Input
            id='capability-rule-name'
            value={props.rule.name || ''}
            onChange={(event) => props.onChange({ name: event.target.value })}
            placeholder={t('Optional display name')}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='capability-selector-type'>
            {t('Selector type')}
          </FieldLabel>
          <Select
            items={selectorTypeItems}
            value={props.rule.selector.type}
            onValueChange={(value) =>
              props.onChange({
                selector: {
                  ...props.rule?.selector,
                  type: (value || 'pattern') as 'pattern' | 'exact',
                  value: props.rule?.selector.value || '',
                },
              })
            }
          >
            <SelectTrigger id='capability-selector-type' className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {selectorTypeItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor='capability-selector-value'>
            {props.rule.selector.type === 'exact'
              ? t('Upstream model')
              : t('Model pattern')}
          </FieldLabel>
          {props.rule.selector.type === 'exact' && props.models.length > 0 ? (
            <Combobox
              id='capability-selector-value'
              options={props.models.map((model) => ({
                value: model,
                label: model,
              }))}
              value={props.rule.selector.value}
              onValueChange={(value) =>
                props.onChange({
                  selector: {
                    ...props.rule?.selector,
                    type: 'exact',
                    value: value || '',
                  },
                })
              }
              allowCustomValue
              openOnFocus
              showAllOptionsOnFocus
              placeholder={t('Model name')}
              className='w-full'
            />
          ) : (
            <Input
              id='capability-selector-value'
              value={props.rule.selector.value}
              onChange={(event) =>
                props.onChange({
                  selector: {
                    ...props.rule?.selector,
                    type: props.rule?.selector.type || 'pattern',
                    value: event.target.value,
                  },
                })
              }
              placeholder='gpt-5*'
            />
          )}
        </Field>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          onClick={props.onDelete}
          aria-label={t('Delete rule')}
        >
          <Trash2 aria-hidden='true' />
        </Button>
      </div>
    </FieldGroup>
  )
}

function ParameterCapabilityRow(props: {
  path: string
  configured?: ParameterCapability
  effective?: { capability: ParameterCapability; source: string }
  onChange: (capability: ParameterCapability | null) => void
}) {
  const { t } = useTranslation()
  const capability = props.configured
  if (!capability) {
    return (
      <div className='flex items-center gap-3 rounded-lg border px-4 py-3'>
        <div className='min-w-0 flex-1'>
          <div className='font-mono text-sm'>{props.path}</div>
          <div className='text-muted-foreground text-xs'>
            {props.effective
              ? t('Inherited from {{source}}', {
                  source:
                    props.effective.source === 'Channel default'
                      ? t('Channel default')
                      : props.effective.source,
                })
              : t('No constraint configured')}
          </div>
        </div>
        {props.effective && (
          <CapabilitySummary capability={props.effective.capability} />
        )}
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => props.onChange({ on_violation: 'reject' })}
        >
          {t('Override')}
        </Button>
      </div>
    )
  }

  let supportValue = 'inherit'
  if (capability.supported !== undefined) {
    supportValue = capability.supported ? 'supported' : 'unsupported'
  }
  const supportItems = SUPPORT_OPTIONS.map((option) => ({
    value: option.value,
    label: t(option.label),
  }))
  const actionItems = ACTION_OPTIONS.map((option) => ({
    value: option.value,
    label: t(option.label),
  }))
  return (
    <div className='flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex items-center gap-3'>
        <div className='min-w-0 flex-1'>
          <div className='font-mono text-sm font-medium'>{props.path}</div>
          <div className='text-muted-foreground text-xs'>
            {props.effective
              ? t('Effective source: {{source}}', {
                  source:
                    props.effective.source === 'Channel default'
                      ? t('Channel default')
                      : props.effective.source,
                })
              : t('Scope override')}
          </div>
        </div>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          onClick={() => props.onChange(null)}
          aria-label={t('Remove override')}
        >
          <Trash2 aria-hidden='true' />
        </Button>
      </div>
      <FieldGroup className='grid grid-cols-2 gap-3 lg:grid-cols-5'>
        <Field>
          <FieldLabel htmlFor={`${props.path}-support`}>
            {t('Support status')}
          </FieldLabel>
          <Select
            items={supportItems}
            value={supportValue}
            onValueChange={(value) => {
              props.onChange({
                ...capability,
                supported:
                  value === 'inherit' ? undefined : value === 'supported',
              })
            }}
          >
            <SelectTrigger id={`${props.path}-support`} className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {supportItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${props.path}-min`}>{t('Minimum')}</FieldLabel>
          <Input
            id={`${props.path}-min`}
            type='number'
            inputMode='decimal'
            step='any'
            value={capability.min ?? ''}
            onChange={(event) =>
              props.onChange({
                ...capability,
                min:
                  event.target.value === ''
                    ? undefined
                    : Number(event.target.value),
              })
            }
            className='[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${props.path}-max`}>{t('Maximum')}</FieldLabel>
          <Input
            id={`${props.path}-max`}
            type='number'
            inputMode='decimal'
            step='any'
            value={capability.max ?? ''}
            onChange={(event) =>
              props.onChange({
                ...capability,
                max:
                  event.target.value === ''
                    ? undefined
                    : Number(event.target.value),
              })
            }
            className='[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${props.path}-allowed`}>
            {t('Allowed values')}
          </FieldLabel>
          <Input
            id={`${props.path}-allowed`}
            value={capability.allowed_values?.join(', ') || ''}
            onChange={(event) =>
              props.onChange({
                ...capability,
                allowed_values: event.target.value
                  .split(',')
                  .map((value) => value.trim())
                  .filter(Boolean),
              })
            }
            placeholder={t('Comma separated')}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${props.path}-action`}>
            {t('On violation')}
          </FieldLabel>
          <Select
            items={actionItems}
            value={capability.on_violation || 'reject'}
            onValueChange={(value) =>
              props.onChange({
                ...capability,
                on_violation: (value || 'reject') as ParameterCapabilityAction,
              })
            }
          >
            <SelectTrigger id={`${props.path}-action`} className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {actionItems.map((item) => (
                  <SelectItem
                    key={item.value}
                    value={item.value}
                    disabled={
                      isBillingSensitiveParameter(props.path) &&
                      item.value !== 'reject'
                    }
                  >
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </FieldGroup>
    </div>
  )
}

function CapabilitySummary(props: { capability: ParameterCapability }) {
  const { t } = useTranslation()
  if (props.capability.supported === false) {
    return <Badge variant='outline'>{t('Unsupported')}</Badge>
  }
  if (
    props.capability.min !== undefined ||
    props.capability.max !== undefined
  ) {
    return (
      <Badge variant='outline'>
        {props.capability.min ?? '−∞'} – {props.capability.max ?? '+∞'}
      </Badge>
    )
  }
  if (props.capability.allowed_values?.length) {
    return (
      <Badge variant='outline'>
        {t('{{count}} allowed value(s)', {
          count: props.capability.allowed_values.length,
        })}
      </Badge>
    )
  }
  return <Badge variant='outline'>{t('Configured')}</Badge>
}

function CapabilityValidationPanel(props: {
  config: ParameterCapabilityConfig
  models: string[]
}) {
  const { t } = useTranslation()
  const [model, setModel] = useState(props.models[0] || '')
  const [requestText, setRequestText] = useState(
    '{\n  "temperature": 1.5,\n  "max_tokens": 4096\n}'
  )
  const [parseError, setParseError] = useState('')
  const [request, setRequest] = useState<Record<string, unknown>>({
    temperature: 1.5,
    max_tokens: 4096,
  })

  const evaluation = evaluateParameterCapabilities(props.config, model, request)

  function runValidation(): void {
    try {
      const parsed: unknown = JSON.parse(requestText)
      if (
        typeof parsed !== 'object' ||
        parsed === null ||
        Array.isArray(parsed)
      ) {
        setParseError(t('Request must be a JSON object.'))
        return
      }
      setRequest(parsed as Record<string, unknown>)
      setParseError('')
    } catch {
      setParseError(t('Request JSON is invalid.'))
    }
  }

  return (
    <div className='grid h-full min-h-0 grid-cols-2'>
      <div className='flex min-h-0 flex-col gap-4 border-r p-5'>
        <div>
          <h3 className='font-medium'>{t('Test a request')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Preview whether a request is accepted and how drop or clamp policies transform it.'
            )}
          </p>
        </div>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor='capability-test-model'>
              {t('Upstream model')}
            </FieldLabel>
            {props.models.length > 0 ? (
              <Combobox
                id='capability-test-model'
                options={props.models.map((item) => ({
                  value: item,
                  label: item,
                }))}
                value={model}
                onValueChange={(value) => setModel(value || '')}
                allowCustomValue
                openOnFocus
                showAllOptionsOnFocus
                placeholder={t('Model name')}
                className='w-full'
              />
            ) : (
              <Input
                id='capability-test-model'
                value={model}
                onChange={(event) => setModel(event.target.value)}
                placeholder={t('Model name')}
              />
            )}
          </Field>
          <Field data-invalid={Boolean(parseError)}>
            <FieldLabel htmlFor='capability-test-request'>
              {t('Request parameters')}
            </FieldLabel>
            <Textarea
              id='capability-test-request'
              value={requestText}
              onChange={(event) => setRequestText(event.target.value)}
              rows={14}
              className='font-mono'
              aria-invalid={Boolean(parseError)}
            />
            <FieldDescription>
              {parseError ||
                t('Enter the parameter portion of a JSON request.')}
            </FieldDescription>
          </Field>
        </FieldGroup>
        <Button type='button' onClick={runValidation}>
          <FlaskConical data-icon='inline-start' />
          {t('Validate request')}
        </Button>
      </div>

      <ScrollArea className='min-h-0 p-5'>
        <div className='flex flex-col gap-4'>
          <div className='flex items-center gap-3'>
            <Badge
              variant={evaluation.compatible ? 'secondary' : 'destructive'}
            >
              {evaluation.compatible ? t('Compatible') : t('Rejected')}
            </Badge>
            <span className='text-muted-foreground text-sm'>
              {t('{{count}} evaluated parameter(s)', {
                count: evaluation.evaluations.length,
              })}
            </span>
          </div>

          {evaluation.evaluations.length === 0 ? (
            <Alert>
              <CircleAlert />
              <AlertTitle>{t('No configured parameters found')}</AlertTitle>
              <AlertDescription>
                {t(
                  'The request does not contain parameters constrained by the selected model.'
                )}
              </AlertDescription>
            </Alert>
          ) : (
            <div className='flex flex-col gap-2'>
              {evaluation.evaluations.map((item) => (
                <div
                  key={item.parameter}
                  className='flex items-start gap-3 rounded-lg border p-3'
                >
                  <Badge
                    variant={
                      item.status === 'rejected' ? 'destructive' : 'outline'
                    }
                  >
                    <CapabilityStatusLabel status={item.status} />
                  </Badge>
                  <div className='min-w-0 flex-1'>
                    <div className='font-mono text-sm'>{item.parameter}</div>
                    <div className='text-muted-foreground text-xs'>
                      <CapabilityEvaluationMessage evaluation={item} />
                    </div>
                    {item.to !== undefined && (
                      <div className='mt-1 text-xs'>
                        {String(item.from)} → {String(item.to)}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}

          <Field>
            <FieldTitle>{t('Transformed request')}</FieldTitle>
            <Textarea
              readOnly
              value={JSON.stringify(evaluation.request, null, 2)}
              rows={12}
              className={cn(
                'font-mono',
                !evaluation.compatible && 'opacity-70'
              )}
            />
          </Field>
        </div>
      </ScrollArea>
    </div>
  )
}

function CapabilityStatusLabel(props: {
  status: CapabilityEvaluation['status']
}) {
  const { t } = useTranslation()
  switch (props.status) {
    case 'compatible':
      return t('Compatible')
    case 'rejected':
      return t('Rejected')
    case 'dropped':
      return t('Dropped')
    case 'clamped':
      return t('Clamped')
  }
}

function CapabilityEvaluationMessage(props: {
  evaluation: CapabilityEvaluation
}) {
  const { t } = useTranslation()
  switch (props.evaluation.reason) {
    case 'compatible':
      return t('Compatible')
    case 'unsupported':
      return t('Parameter is not supported')
    case 'number_required':
      return t('Value must be a number')
    case 'minimum':
      return t('Value must be at least {{value}}', {
        value: props.evaluation.constraint,
      })
    case 'maximum':
      return t('Value must be at most {{value}}', {
        value: props.evaluation.constraint,
      })
    case 'allowed_values':
      return t('Value must be one of: {{values}}', {
        values: props.evaluation.constraint,
      })
  }
}

function CapabilityConfigErrorMessage(props: {
  error: ParameterCapabilityConfigError
}) {
  const { t } = useTranslation()
  switch (props.error.code) {
    case 'selector_required':
      return t('{{scope}}: model selector is required', {
        scope: props.error.scope,
      })
    case 'invalid_path':
      return t('{{scope}}: invalid parameter path {{path}}', {
        scope: props.error.scope,
        path: props.error.path,
      })
    case 'inverted_range':
      return t('{{scope}}: {{path}} minimum cannot exceed maximum', {
        scope: props.error.scope,
        path: props.error.path,
      })
    case 'clamp_without_boundary':
      return t('{{scope}}: {{path}} cannot clamp without a numeric boundary', {
        scope: props.error.scope,
        path: props.error.path,
      })
    case 'unsafe_billing_action':
      return t(
        '{{scope}}: {{path}} affects billing and must reject incompatible values',
        {
          scope: props.error.scope,
          path: props.error.path,
        }
      )
  }
}
