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

import { BUSINESS_PRESETS, formatTokenCount, type TokenCostInput } from '../lib'

type BusinessPresetsProps = {
  onSelect: (value: TokenCostInput) => void
}

export function BusinessPresets(props: BusinessPresetsProps): ReactElement {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Common business presets')}</CardTitle>
        <CardDescription>
          {t(
            'Typical per-request token volumes. Select a preset to update the calculator.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {BUSINESS_PRESETS.map((preset) => (
          <button
            type='button'
            key={preset.labelKey}
            onClick={() =>
              props.onSelect({
                totalTokens: preset.totalTokens,
                inputRatio: preset.inputRatio,
                outputRatio: preset.outputRatio,
                cacheHitRate: preset.cacheHitRate,
                cacheWriteRate: preset.cacheWriteRate,
              })
            }
            className='bg-muted/40 hover:bg-muted rounded-lg border p-4 text-left transition-colors'
          >
            <p className='font-medium'>{t(preset.labelKey)}</p>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                '{{tokens}} tokens · {{input}}:{{output}} input/output · {{hit}}% cache hit',
                {
                  tokens: formatTokenCount(preset.totalTokens),
                  input: preset.inputRatio,
                  output: preset.outputRatio,
                  hit: preset.cacheHitRate,
                }
              )}
            </p>
          </button>
        ))}
      </CardContent>
    </Card>
  )
}
