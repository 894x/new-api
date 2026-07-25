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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { PricingData, PricingModel } from '../../pricing/types'
import type { TokenCostInput } from '../lib'
import { Field, NumberField } from './Fields'

type CalculatorFormProps = {
  models: PricingModel[]
  model: PricingModel
  groups: string[]
  group: string
  pricing: PricingData
  input: TokenCostInput
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  onInputChange: (key: keyof TokenCostInput, value: string) => void
}

export function CalculatorForm(props: CalculatorFormProps): ReactElement {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Estimate usage cost')}</CardTitle>
        <CardDescription>
          {t('Prices follow the platform currency and base recharge settings.')}
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
        <NumberField
          label={t('Total tokens')}
          value={props.input.totalTokens}
          onChange={(value) => props.onInputChange('totalTokens', value)}
        />
        <div className='grid grid-cols-2 gap-3'>
          <NumberField
            label={t('Input ratio')}
            value={props.input.inputRatio}
            onChange={(value) => props.onInputChange('inputRatio', value)}
          />
          <NumberField
            label={t('Output ratio')}
            value={props.input.outputRatio}
            onChange={(value) => props.onInputChange('outputRatio', value)}
          />
        </div>
        <NumberField
          label={t('Cache hit rate')}
          suffix='%'
          max={100}
          value={props.input.cacheHitRate}
          onChange={(value) => props.onInputChange('cacheHitRate', value)}
        />
        <NumberField
          label={t('Cache write rate')}
          suffix='%'
          max={100 - props.input.cacheHitRate}
          value={props.input.cacheWriteRate}
          onChange={(value) => props.onInputChange('cacheWriteRate', value)}
        />
      </CardContent>
    </Card>
  )
}
