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
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { PricingData, PricingModel } from '../../pricing/types'
import {
  TOKEN_VOLUME_PRESETS,
  USAGE_SCENARIOS,
  type TokenCostInput,
} from '../lib'
import { Field, NumberField } from './Fields'

export type CalculatorMode = 'cost' | 'budget'

type CalculatorFormProps = {
  models: PricingModel[]
  model: PricingModel
  groups: string[]
  group: string
  pricing: PricingData
  mode: CalculatorMode
  scenarioId: string
  budget: number
  input: TokenCostInput
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  onScenarioChange: (value: string) => void
  onInputChange: (key: keyof TokenCostInput, value: string) => void
  onBudgetChange: (value: string) => void
}

export function CalculatorForm(props: CalculatorFormProps): ReactElement {
  const { t } = useTranslation()
  const tokenVolumeOptions = TOKEN_VOLUME_PRESETS.map((preset) => ({
    value: String(preset.value),
    label: t(preset.labelKey),
  }))
  const selectedScenario =
    USAGE_SCENARIOS.find(
      (scenario) => scenario.labelKey === props.scenarioId
    ) ?? USAGE_SCENARIOS[0]
  return (
    <Card className='overflow-visible'>
      <CardHeader>
        <CardTitle>
          {props.mode === 'cost'
            ? t('Estimate usage cost')
            : t('Budget estimate')}
        </CardTitle>
        <CardDescription>
          {props.mode === 'cost'
            ? t('Enter expected usage to estimate customer payment.')
            : t('Enter a budget to estimate the token volume it can cover.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-4 sm:grid-cols-2'>
        <Field label={t('Model')}>
          <Select
            value={props.model.model_name}
            onValueChange={(value) => props.onModelChange(value ?? '')}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {props.models.map((item) => (
                <SelectItem key={item.model_name} value={item.model_name}>
                  {item.model_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('Pricing group')}>
          <Select
            value={props.group}
            onValueChange={(value) => props.onGroupChange(value ?? '')}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {props.groups.map((name) => (
                <SelectItem key={name} value={name}>
                  {props.pricing.usable_group[name]?.desc || name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={`${t('Usage scenario')} / ${t('Cache hit rate')}`}>
          <Select
            value={props.scenarioId}
            onValueChange={(value) => props.onScenarioChange(value ?? '')}
          >
            <SelectTrigger className='w-full'>
              <SelectValue>
                <span className='flex w-full items-center justify-between gap-4'>
                  <span>{t(selectedScenario.labelKey)}</span>
                  <span className='text-muted-foreground ml-auto'>
                    {selectedScenario.cacheHitRate}%
                  </span>
                </span>
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {USAGE_SCENARIOS.map((scenario) => (
                <SelectItem key={scenario.id} value={scenario.labelKey}>
                  <span className='flex w-full items-center justify-between gap-4'>
                    <span>{t(scenario.labelKey)}</span>
                    <span className='text-muted-foreground ml-auto'>
                      {scenario.cacheHitRate}%
                    </span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {props.mode === 'cost' ? (
          <Field label={t('Token volume')}>
            <Combobox
              options={tokenVolumeOptions}
              value={String(props.input.totalTokens)}
              onValueChange={(value) =>
                props.onInputChange('totalTokens', value ?? '')
              }
              placeholder={t('Enter a token volume')}
              emptyText={t('Custom value')}
              allowCustomValue
              showAllOptionsOnFocus
            />
          </Field>
        ) : (
          <NumberField
            label={t('Budget')}
            value={props.budget}
            onChange={props.onBudgetChange}
          />
        )}
      </CardContent>
    </Card>
  )
}
