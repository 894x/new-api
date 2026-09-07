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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'

import type { MatrixData } from '../types'

export function createMatrixFixture(): MatrixData {
  return {
    models: ['model-a', 'model-b'],
    channels: [
      { id: 7, name: 'Alpha', type: 1, status: 1, groups: ['default'] },
      { id: 8, name: 'Beta', type: 1, status: 1, groups: ['premium'] },
    ],
    cells: [
      {
        channel_id: 7,
        channel_name: 'Alpha',
        channel_type: 1,
        channel_status: 1,
        model: 'model-a',
        default_rpm: 500,
        default_tpm: 10000,
        default_priority: 10,
        default_weight: 20,
        rpm_override: null,
        tpm_override: 0,
        priority_override: null,
        weight_override: null,
        effective_rpm: 500,
        effective_tpm: 0,
        effective_priority: 10,
        effective_weight: 20,
        upstream_model: 'provider-a',
        mapping_error: false,
      },
      {
        channel_id: 8,
        channel_name: 'Beta',
        channel_type: 1,
        channel_status: 1,
        model: 'model-b',
        default_rpm: 0,
        default_tpm: 0,
        default_priority: 0,
        default_weight: 0,
        rpm_override: null,
        tpm_override: null,
        priority_override: null,
        weight_override: null,
        effective_rpm: 0,
        effective_tpm: 0,
        effective_priority: 0,
        effective_weight: 0,
        upstream_model: 'model-b',
        mapping_error: false,
      },
    ],
    groups: ['default', 'premium'],
    model_total: 2,
    channel_total: 2,
    model_page: 1,
    channel_page: 1,
    model_page_size: 25,
    channel_page_size: 10,
  }
}

export function createMatrixQueryFixture() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  function Wrapper(props: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, props.children)
  }
  return { client, wrapper: Wrapper }
}
