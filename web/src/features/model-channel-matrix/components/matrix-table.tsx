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
import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { getModelRoutingOverrideKey } from '@/features/channels/lib/model-routing-overrides'
import { toIntlLocale } from '@/i18n/languages'

import type { MatrixCell, MatrixData } from '../types'

function MatrixCellValues(props: { cell: MatrixCell; id: string }) {
  const { t, i18n } = useTranslation()
  const fields = [
    {
      label: t('RPM'),
      value: props.cell.effective_rpm ?? 0,
      override: props.cell.rpm_override,
      capacity: true,
    },
    {
      label: t('TPM'),
      value: props.cell.effective_tpm ?? 0,
      override: props.cell.tpm_override,
      capacity: true,
    },
    {
      label: t('Priority'),
      value: props.cell.effective_priority,
      override: props.cell.priority_override,
      capacity: false,
    },
    {
      label: t('Weight'),
      value: props.cell.effective_weight,
      override: props.cell.weight_override,
      capacity: false,
    },
  ]
  return (
    <span
      id={props.id}
      className='grid min-w-0 flex-1 grid-cols-[minmax(0,4fr)_minmax(0,6fr)] gap-x-1.5 gap-y-1 text-xs'
    >
      {fields.map((field) => {
        const value =
          field.capacity && field.value === 0
            ? t('Unlimited')
            : field.value.toLocaleString(toIntlLocale(i18n.language))
        const source = field.override == null ? t('Inherit') : t('Override')
        return (
          <span
            key={field.label}
            className='flex min-w-0 items-baseline justify-between gap-0.5'
            title={`${field.label}: ${value} (${source})`}
          >
            <span className='text-muted-foreground shrink-0 font-normal'>
              {field.label}
            </span>
            <span className='flex min-w-0 items-baseline gap-0.5 font-semibold tabular-nums'>
              <span className='truncate'>{value}</span>
              {field.override != null && (
                <span aria-hidden='true' className='text-primary shrink-0'>
                  *
                </span>
              )}
            </span>
            <span className='sr-only'> {source}</span>
          </span>
        )
      })}
    </span>
  )
}

export function MatrixTable(props: {
  data: MatrixData
  busy: boolean
  onSelect: (cell: MatrixCell, trigger: HTMLElement) => void
}) {
  const { t } = useTranslation()
  const tableId = useId()
  const cells = useMemo(
    () =>
      new Map(
        props.data.cells.map((cell) => [
          getModelRoutingOverrideKey(cell.channel_id, cell.model),
          cell,
        ])
      ),
    [props.data.cells]
  )

  return (
    <div
      role='region'
      aria-label={t('Model-channel matrix')}
      aria-busy={props.busy}
      tabIndex={0}
      className='focus-visible:outline-ring min-h-0 min-w-0 flex-1 overflow-auto rounded-lg border focus-visible:outline-2'
    >
      {/* A single scroll container keeps both sticky axes in the same viewport. */}
      <table
        className='table-fixed border-separate border-spacing-0 text-xs'
        style={{ width: `${8 + props.data.channels.length * 12}rem` }}
      >
        <caption className='sr-only'>
          {t('Models as rows, channels as columns.')}
        </caption>
        <colgroup>
          <col className='w-32' />
          {props.data.channels.map((channel) => (
            <col key={channel.id} className='w-48' />
          ))}
        </colgroup>
        <thead>
          <tr>
            <th
              scope='col'
              className='bg-muted sticky top-0 left-0 z-30 border-r border-b px-2 py-1.5 text-left'
            >
              {t('Model')}
              <span
                aria-hidden='true'
                className='text-muted-foreground mt-1 block font-normal'
              >
                <span aria-hidden='true' className='text-primary'>
                  *
                </span>{' '}
                {t('Override')}
              </span>
            </th>
            {props.data.channels.map((channel) => (
              <th
                key={channel.id}
                scope='col'
                className='bg-muted sticky top-0 z-20 border-r border-b px-2 py-1.5 text-left align-top'
              >
                <div className='flex items-start justify-between gap-2'>
                  <span className='min-w-0 truncate' title={channel.name}>
                    {channel.name}
                  </span>
                  <span className='text-muted-foreground shrink-0 font-normal'>
                    #{channel.id}
                  </span>
                </div>
                <div className='mt-1 flex min-w-0 items-center gap-1'>
                  <Badge
                    variant={channel.status === 1 ? 'secondary' : 'outline'}
                  >
                    {channel.status === 1 ? t('Enabled') : t('Disabled')}
                  </Badge>
                  <span
                    className='text-muted-foreground min-w-0 truncate font-normal'
                    title={channel.groups.join(', ')}
                  >
                    {channel.groups.join(', ')}
                  </span>
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {props.data.models.map((model, rowIndex) => (
            <tr key={model}>
              <th
                scope='row'
                className='bg-background sticky left-0 z-10 border-r border-b px-2 py-1 text-left font-medium'
              >
                <span className='block truncate' title={model}>
                  {model}
                </span>
              </th>
              {props.data.channels.map((channel) => {
                const cell = cells.get(
                  getModelRoutingOverrideKey(channel.id, model)
                )
                const descriptionId = `${tableId}-${channel.id}-${rowIndex}`
                return (
                  <td
                    key={channel.id}
                    className='border-r border-b p-0.5 align-middle'
                  >
                    {cell ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='xs'
                        className='h-12 w-full min-w-0 justify-start px-1.5 py-1 text-left'
                        aria-label={t(
                          'Configure {{model}} on {{channel}} (#{{id}})',
                          { model, channel: channel.name, id: channel.id }
                        )}
                        aria-haspopup='dialog'
                        aria-describedby={descriptionId}
                        disabled={props.busy}
                        onClick={(event) =>
                          props.onSelect(cell, event.currentTarget)
                        }
                      >
                        <MatrixCellValues cell={cell} id={descriptionId} />
                      </Button>
                    ) : (
                      <div
                        className='text-muted-foreground flex h-12 items-center justify-center'
                        title={t('Not configured')}
                      >
                        <span aria-hidden='true'>—</span>
                        <span className='sr-only'>{t('Not configured')}</span>
                      </div>
                    )}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
