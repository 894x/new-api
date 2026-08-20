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
import { ListCollapse } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export type DataTableDensityToggleProps = {
  compact: boolean
  onChange: (compact: boolean) => void
  className?: string
}

export function DataTableDensityToggle(props: DataTableDensityToggleProps) {
  const { t } = useTranslation()
  const nextModeLabel = props.compact ? t('Comfortable') : t('Compact')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type='button'
            variant={props.compact ? 'secondary' : 'outline'}
            size='icon'
            aria-label={nextModeLabel}
            aria-pressed={props.compact}
            onClick={() => props.onChange(!props.compact)}
            className={cn('size-8', props.className)}
          >
            <ListCollapse />
          </Button>
        }
      />
      <TooltipContent side='bottom' className='text-xs'>
        {nextModeLabel}
      </TooltipContent>
    </Tooltip>
  )
}
