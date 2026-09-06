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
import { render } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { UsageCell } from '../usage-cell'
import type { TaskUsage } from '../../types'

describe('usage cell', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Cache: 'Cache',
    })
  })

  test('keeps token input, output, and cache usage visible', () => {
    const rendered = render(
      <UsageCell
        promptTokens={1200}
        completionTokens={300}
        cacheReadTokens={400}
        cacheWriteTokens={20}
      />
    )

    const text = (rendered.container.textContent ?? '').replaceAll(/\s/g, '')
    expect(text).toContain('1,200/300')
    expect(text).toContain('Cache↓400')
    expect(text).toContain('↑20')
  })

  test('shows input image count alongside video duration usage', () => {
    const rendered = render(
      <UsageCell
        usage={{
          kind: 'video_duration',
          unit: 'second',
          input: 2,
          output: 4,
          total: 6,
          input_images: 7,
        } as TaskUsage}
      />
    )

    const text = (rendered.container.textContent ?? '').replaceAll(/\s/g, '')
    expect(text).toContain('Image7')
  })
})
