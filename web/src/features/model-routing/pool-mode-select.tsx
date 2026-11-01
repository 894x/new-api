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
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { RoutingMode } from './api'

export function PoolModeSelect(props: {
  value: RoutingMode
  onChange: (mode: RoutingMode) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Select
      value={props.value}
      onValueChange={(value) => {
        if (value === 'priority' || value === 'smart') props.onChange(value)
      }}
      disabled={props.disabled}
    >
      <SelectTrigger
        aria-label={t('Scheduling strategy')}
        className='w-full sm:w-44'
      >
        <SelectValue>
          {props.value === 'smart' ? t('Smart routing') : t('Priority routing')}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value='priority'>{t('Priority routing')}</SelectItem>
        <SelectItem value='smart'>{t('Smart routing')}</SelectItem>
      </SelectContent>
    </Select>
  )
}
