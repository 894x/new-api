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
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import { AxiosError } from 'axios'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { ModelPricingEditorPanel } from '@/features/system-settings/models/model-pricing-sheet'
import { ModelRatioVisualEditor } from '@/features/system-settings/models/model-ratio-visual-editor'
import { useCommonLogsColumns } from '@/features/usage-logs/components/columns/common-logs-columns'
import { DetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { api } from '@/lib/http-client'
import { createAppQueryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth-store'

import { usePricingData } from '../hooks/use-pricing-data'

const originalAdapter = api.defaults.adapter
let client: QueryClient
let requests: string[]

const log: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: 'administrator',
  token_name: 'token',
  model_name: 'example',
  quota: 5000,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 1,
  channel_name: '',
  token_id: 1,
  group: 'default',
  ip: '',
  other: JSON.stringify({
    is_task: true,
    billing_mode: 'tiered_expr',
    expr_b64: btoa('tier("images", u("images") * 0.25)'),
    matched_tier: 'images',
    usage_facts: { images: 1 },
  }),
  request_id: '',
  upstream_request_id: '',
}

function LogPricePreview() {
  const table = useReactTable({
    data: [log],
    columns: useCommonLogsColumns(true, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'content')
  if (!cell) throw new Error('The log must have a content column')
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}

function Wrapper(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

beforeEach(() => {
  requests = []
  localStorage.clear()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'administrator', role: 100 })
  client = createAppQueryClient()
  vi.spyOn(toast, 'error').mockReturnValue('pricing-error')
  api.defaults.adapter = async (config) => {
    requests.push(config.url ?? '')
    let data: unknown
    if (config.url === '/api/pricing') {
      throw new AxiosError(
        'Request failed with status code 403',
        'ERR_BAD_REQUEST',
        config,
        undefined,
        {
          data: { success: false, message: 'pricing is disabled' },
          status: 403,
          statusText: 'Forbidden',
          headers: {},
          config,
        }
      )
    }
    if (config.url === '/api/status') {
      data = {
        success: true,
        data: { HeaderNavModules: { pricing: { enabled: false } } },
      }
    } else if (config.url === '/api/user/self/pricing') {
      data = {
        success: true,
        data: [
          {
            model_name: 'example',
            billing_usage_schema: {
              images: {
                type: 'number',
                unit: 'count',
                unitLabel: { en: 'image' },
              },
            },
          },
        ],
        vendors: [],
        group_ratio: { default: 1 },
      }
    } else if (config.url === '/api/option/model_pricing/preview') {
      data = {
        success: true,
        data: {
          effective: { ModelRatio: 1, CompletionRatio: 2 },
          cache_write_mode: 'none',
        },
      }
    } else {
      throw new Error(`Unexpected request: ${config.url}`)
    }
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.setUser(null)
  localStorage.clear()
  vi.restoreAllMocks()
})

it.each(['editor', 'table'] as const)(
  'loads account prices in the administration %s with the model square disabled',
  async (view) => {
    render(
      view === 'editor' ? (
        <ModelPricingEditorPanel
          editData={{
            name: 'example',
            billingMode: 'per-token',
            ratio: '1',
            completionRatio: '2',
          }}
        />
      ) : (
        <ModelRatioVisualEditor
          savedModelPrice='{}'
          savedModelRatio='{"example":1}'
          savedCacheRatio='{}'
          savedCreateCacheRatio='{}'
          savedCompletionRatio='{}'
          savedImageRatio='{}'
          savedAudioRatio='{}'
          savedAudioCompletionRatio='{}'
          savedBillingMode='{}'
          savedBillingExpr='{}'
          modelPrice='{}'
          modelRatio='{"example":1}'
          cacheRatio='{}'
          createCacheRatio='{}'
          completionRatio='{}'
          imageRatio='{}'
          audioRatio='{}'
          audioCompletionRatio='{}'
          billingMode='{}'
          billingExpr='{}'
          onChange={vi.fn()}
          onSave={vi.fn()}
          isSaving={false}
        />
      ),
      { wrapper: Wrapper }
    )
    await waitFor(() => expect(requests).toContain('/api/user/self/pricing'))
    await waitFor(() => expect(client.isFetching()).toBe(0))
    expect(requests).not.toContain('/api/pricing')
    expect(toast.error).not.toHaveBeenCalled()
    if (view === 'editor') {
      expect(screen.getByRole('textbox', { name: 'Input price' })).toBeVisible()
    } else {
      expect(screen.getByText('example')).toBeVisible()
    }
  }
)

it.each(['preview', 'dialog'] as const)(
  'loads price units in the log %s with the model square disabled',
  async (view) => {
    render(
      view === 'preview' ? (
        <LogPricePreview />
      ) : (
        <DetailsDialog
          log={log}
          isAdmin
          isRoot={false}
          open
          onOpenChange={vi.fn()}
        />
      ),
      { wrapper: Wrapper }
    )
    const prices = await screen.findAllByText(/\$0\.25\/image/)
    for (const price of prices) expect(price).toBeVisible()
    await waitFor(() => expect(client.isFetching()).toBe(0))
    expect(requests).toContain('/api/user/self/pricing')
    expect(requests).not.toContain('/api/pricing')
    expect(toast.error).not.toHaveBeenCalled()
  }
)

it('keeps public catalog price queries blocked when the model square is disabled', async () => {
  const { result } = renderHook(() => usePricingData(), { wrapper: Wrapper })
  await waitFor(() => expect(result.current.error).toBeTruthy())
  expect(requests).toContain('/api/pricing')
  expect(requests).not.toContain('/api/user/self/pricing')
  expect(toast.error).toHaveBeenCalledWith('pricing is disabled')
})
