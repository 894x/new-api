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
import type { ReactElement, ReactNode } from 'react'

import { Input } from '@/components/ui/input'

type FieldProps = {
  label: string
  children: ReactNode
}

export function Field(props: FieldProps): ReactElement {
  return (
    <label className='grid gap-1.5 text-sm font-medium'>
      <span>{props.label}</span>
      {props.children}
    </label>
  )
}

export function NumberField(props: {
  label: string
  value: number
  suffix?: string
  max?: number
  onChange: (value: string) => void
}): ReactElement {
  return (
    <label className='grid gap-1.5 text-sm font-medium'>
      <span>{props.label}</span>
      <div className='relative'>
        <Input
          type='number'
          min='0'
          max={props.max}
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
          className={props.suffix ? 'pr-8' : undefined}
        />
        {props.suffix && (
          <span className='text-muted-foreground absolute inset-y-0 right-3 flex items-center text-sm'>
            {props.suffix}
          </span>
        )}
      </div>
    </label>
  )
}

export function Metric(props: { label: string; value: string }): ReactElement {
  return (
    <div className='bg-background/60 rounded-lg border px-3 py-2.5'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='mt-1 font-medium'>{props.value}</p>
    </div>
  )
}
