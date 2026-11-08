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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  earlyChunkModeSchema,
  type EarlyChunkMode,
  type EarlyChunkPool,
} from './early-chunk'

export function EarlyChunkModeSelect(props: {
  value?: EarlyChunkMode
  onChange: (value: EarlyChunkMode) => void
  label: string
  disabled?: boolean
  inherit?: 'global' | 'pool' | false
}) {
  const { t } = useTranslation()
  const id = useId()
  let label = t('Inherit global')
  if (props.inherit === 'pool') label = t('Inherit pool')
  if (props.value === 'off') label = t('Disabled')
  if (props.value === 'observe') label = t('Observe only')
  if (props.value === 'enabled') label = t('Enabled')
  return (
    <div className='space-y-1.5'>
      <Label htmlFor={id}>{props.label}</Label>
      <Select
        value={props.value || 'inherit'}
        disabled={props.disabled}
        onValueChange={(value) =>
          props.onChange(
            earlyChunkModeSchema.parse(value === 'inherit' ? '' : value)
          )
        }
      >
        <SelectTrigger id={id} className='w-full'>
          <SelectValue>{label}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {props.inherit !== false && (
              <SelectItem value='inherit'>
                {props.inherit === 'pool'
                  ? t('Inherit pool')
                  : t('Inherit global')}
              </SelectItem>
            )}
            <SelectItem value='off'>{t('Disabled')}</SelectItem>
            <SelectItem value='observe'>{t('Observe only')}</SelectItem>
            <SelectItem value='enabled'>{t('Enabled')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

// Pool-level defaults shared by the default-pool and user-pool editors.
export function EarlyChunkSettings(props: {
  value?: EarlyChunkPool | null
  disabled: boolean
  onChange: (value: EarlyChunkPool) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <EarlyChunkModeSelect
        label={t('Early empty chunk')}
        value={props.value?.mode}
        disabled={props.disabled}
        onChange={(mode) =>
          props.onChange({ ...props.value, mode: mode || undefined })
        }
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Streaming Chat Completions only. Observation records predictions and actual content gaps without sending an empty chunk. Each pool and its channel overrides are independent.'
        )}
      </p>
    </div>
  )
}
