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

import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { GroupModelAccess, GroupModelMatrixData } from '../group-model-api'

export function GroupModelTable(props: {
  data: GroupModelMatrixData
  groups: string[]
  models: string[]
  config: GroupModelAccess
  disabled: boolean
  onChange: (group: string, models: string[] | null) => void
}) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      className='min-h-0 min-w-0 flex-1 overflow-auto'
      containerProps={{
        role: 'region',
        'aria-label': t('Group-model matrix'),
        tabIndex: 0,
      }}
      tableClassName='table-fixed border-separate border-spacing-0'
      tableProps={{
        withContainer: false,
        style: { width: `${16 + props.models.length * 11}rem` },
      }}
    >
      <caption className='sr-only'>
        {t('Groups as rows, models as columns.')}
      </caption>
      <colgroup>
        <col className='w-64' />
        {props.models.map((model) => (
          <col key={model} className='w-44' />
        ))}
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead
            scope='col'
            className='bg-muted sticky top-0 left-0 z-30 border-r border-b'
          >
            {t('Group')}
          </TableHead>
          {props.models.map((model) => (
            <TableHead
              key={model}
              scope='col'
              className='bg-muted sticky top-0 z-20 border-r border-b'
            >
              <span className='block truncate' title={model}>
                {model}
              </span>
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.groups.map((group) => {
          const restricted = Object.hasOwn(props.config, group)
          const selected = new Set(
            restricted ? props.config[group] : props.data.models
          )
          const available = new Set(
            Object.hasOwn(props.data.available, group)
              ? props.data.available[group]
              : []
          )
          return (
            <TableRow key={group}>
              <TableHead
                scope='row'
                className='bg-background sticky left-0 z-10 border-r border-b py-3'
              >
                <span className='block truncate' title={group}>
                  {group}
                </span>
                <div className='text-muted-foreground mt-2 flex items-center gap-2 text-xs font-normal'>
                  <Switch
                    size='sm'
                    checked={restricted}
                    disabled={props.disabled}
                    aria-label={t('Restrict models for {{group}}', { group })}
                    onCheckedChange={(checked) =>
                      props.onChange(
                        group,
                        checked ? [...props.data.models] : null
                      )
                    }
                  />
                  {t('Selected models only')}
                </div>
                {restricted && (
                  <div className='mt-1 flex gap-1'>
                    <Button
                      size='xs'
                      variant='ghost'
                      disabled={props.disabled}
                      onClick={() =>
                        props.onChange(group, [...props.data.models])
                      }
                    >
                      {t('Select all')}
                    </Button>
                    <Button
                      size='xs'
                      variant='ghost'
                      disabled={props.disabled}
                      onClick={() => props.onChange(group, [])}
                    >
                      {t('Clear')}
                    </Button>
                  </div>
                )}
              </TableHead>
              {props.models.map((model) => (
                <TableCell
                  key={model}
                  className='border-r border-b text-center'
                >
                  <div className='flex flex-col items-center gap-2'>
                    <Checkbox
                      checked={selected.has(model)}
                      disabled={props.disabled}
                      aria-label={t('Allow {{group}} to use {{model}}', {
                        group,
                        model,
                      })}
                      onCheckedChange={(checked) => {
                        const next = new Set(selected)
                        if (checked) next.add(model)
                        else next.delete(model)
                        props.onChange(group, [...next].sort())
                      }}
                    />
                    {!available.has(model) && (
                      <span className='text-muted-foreground text-xs'>
                        {t('No channel')}
                      </span>
                    )}
                  </div>
                </TableCell>
              ))}
            </TableRow>
          )
        })}
      </TableBody>
    </StaticDataTable>
  )
}
