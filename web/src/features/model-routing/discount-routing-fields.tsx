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

import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'

export function DiscountRoutingFields(props: {
  maxTTFT: number
  maxTPOT: number
  disabled?: boolean
  invalidTTFT?: boolean
  invalidTPOT?: boolean
  onChange: (field: 'max_ttft_ms' | 'max_tpot_ms', value: number) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <FieldGroup>
      <Field data-invalid={props.invalidTTFT || undefined}>
        <FieldLabel htmlFor={`${id}-ttft`}>
          {t('Maximum first-token latency (ms)')}
        </FieldLabel>
        <Input
          id={`${id}-ttft`}
          type='number'
          min={1}
          max={3600000}
          step={1}
          required
          disabled={props.disabled}
          aria-invalid={props.invalidTTFT || undefined}
          value={Number.isFinite(props.maxTTFT) ? props.maxTTFT : ''}
          onChange={(event) =>
            props.onChange('max_ttft_ms', event.target.valueAsNumber)
          }
        />
      </Field>
      <Field data-invalid={props.invalidTPOT || undefined}>
        <FieldLabel htmlFor={`${id}-tpot`}>
          {t('Maximum time per output token (ms)')}
        </FieldLabel>
        <Input
          id={`${id}-tpot`}
          type='number'
          min={1}
          max={60000}
          step={1}
          required
          disabled={props.disabled}
          aria-invalid={props.invalidTPOT || undefined}
          value={Number.isFinite(props.maxTPOT) ? props.maxTPOT : ''}
          onChange={(event) =>
            props.onChange('max_tpot_ms', event.target.valueAsNumber)
          }
        />
      </Field>
      <FieldDescription>
        {t(
          'Prefer the lowest configured discount while latency meets these limits. Use more expensive channels when quality degrades, then gradually return after recovery. Unknown discounts rank last. Non-streaming and media requests use pool priorities.'
        )}
      </FieldDescription>
    </FieldGroup>
  )
}
