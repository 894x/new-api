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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { updateSystemOption } from '../../api'
import { SettingsPageProvider } from '../../components/settings-page-context'
import { RatioSettingsCard } from '../ratio-settings-card'

vi.mock('@/features/channels/api', () => ({
  getEnabledModels: vi.fn().mockResolvedValue({
    success: true,
    data: ['gpt-5'],
  }),
}))

vi.mock('../../api', () => ({
  resetModelRatios: vi.fn(),
  updateSystemOption: vi.fn(),
}))

vi.mock('../group-ratio-visual-editor', () => ({
  GroupRatioVisualEditor: () => <div>Group ratio editor</div>,
}))

vi.mock('../group-special-usable-editor', () => ({
  GroupSpecialUsableRulesEditor: () => <div>Special usable rules</div>,
}))

const modelDefaults = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
}

const groupDefaults = {
  GroupRatio: '{"premium":1}',
  TopupGroupRatio: '{}',
  UserUsableGroups: '{}',
  GroupGroupRatio: '{}',
  AutoGroups: '[]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
  ModelTieredRatios: '{}',
}

function renderRatioSettingsCard(modelTieredRatios: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  const actionsContainer = document.createElement('div')
  actionsContainer.dataset.ratioSettingsActions = 'true'
  document.body.appendChild(actionsContainer)

  render(
    <QueryClientProvider client={queryClient}>
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <RatioSettingsCard
          modelDefaults={modelDefaults}
          groupDefaults={{
            ...groupDefaults,
            ModelTieredRatios: modelTieredRatios,
          }}
          toolPricesDefault='{}'
          visibleTabs={['groups']}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )

  return queryClient
}

describe('ratio settings card tiered-discount persistence', () => {
  afterEach(() => {
    document.querySelector('[data-ratio-settings-actions]')?.remove()
    vi.clearAllMocks()
  })

  test('preserves raw duplicate keys from defaults so JSON mode can repair them', async () => {
    const user = userEvent.setup()
    const policy =
      '{"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":1}]}'
    const rawValue = `{"premium":{"gpt-5":${policy}},"premium":{"claude-sonnet-4":${policy}}}`

    renderRatioSettingsCard(rawValue)

    expect(
      screen.getByText(
        'Tiered discount JSON cannot be shown visually. Switch to JSON mode to fix it.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))

    expect(
      screen.getByRole('textbox', { name: 'Monthly tiered discounts' })
    ).toHaveValue(rawValue)
  })

  test('shows a legacy valid value as canonical wrapper JSON', async () => {
    const user = userEvent.setup()
    const policy = {
      enabled: false,
      effective_from: 1_788_192_000,
      effective_until: null,
      timezone: 'UTC',
      tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
    }
    renderRatioSettingsCard(JSON.stringify({ premium: { 'gpt-5': policy } }))

    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))

    const value = screen.getByRole('textbox', {
      name: 'Monthly tiered discounts',
    })
    expect(JSON.parse((value as HTMLTextAreaElement).value)).toEqual({
      premium: {
        progress_basis: 'original',
        models: { 'gpt-5': policy },
      },
    })
  })

  test('documents the canonical wrapper and charged progress in JSON mode', async () => {
    const user = userEvent.setup()
    renderRatioSettingsCard('{}')

    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))

    expect(
      screen.getByText(
        'Canonical shape: { group: { progress_basis, models } }. Use progress_basis: "charged" to advance thresholds by the high-precision settled amount after the current tier discount; "original" uses the original model price.'
      )
    ).toBeInTheDocument()
  })

  test('keeps partial-failure drafts and invalidates options only after a complete retry', async () => {
    vi.mocked(updateSystemOption)
      .mockResolvedValueOnce({ success: true, message: '' })
      .mockResolvedValueOnce({ success: false, message: 'rejected' })
      .mockResolvedValue({ success: true, message: '' })
    const user = userEvent.setup()
    const queryClient = renderRatioSettingsCard('{}')
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
    const groupRatio = screen.getByRole('textbox', { name: 'Group ratios' })
    const topupRatio = screen.getByRole('textbox', {
      name: 'Top-up group ratios',
    })
    const tieredRatios = screen.getByRole('textbox', {
      name: 'Monthly tiered discounts',
    })
    const nextTieredRatios =
      '{"premium":{"gpt-5":{"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":1}]}}}'

    fireEvent.input(groupRatio, { target: { value: '{"premium":0.9}' } })
    fireEvent.input(topupRatio, { target: { value: '{"premium":1.1}' } })
    fireEvent.input(tieredRatios, { target: { value: nextTieredRatios } })
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    await waitFor(() => expect(updateSystemOption).toHaveBeenCalledTimes(2))
    expect(invalidateQueries).not.toHaveBeenCalled()
    expect(groupRatio).toHaveValue('{"premium":0.9}')
    expect(topupRatio).toHaveValue('{"premium":1.1}')
    expect(tieredRatios).toHaveValue(nextTieredRatios)

    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    await waitFor(() => expect(updateSystemOption).toHaveBeenCalledTimes(4))
    expect(
      vi.mocked(updateSystemOption).mock.calls.map(([request]) => request.key)
    ).toEqual([
      'GroupRatio',
      'TopupGroupRatio',
      'TopupGroupRatio',
      'group_ratio_setting.model_tiered_ratios',
    ])
    expect(invalidateQueries).toHaveBeenCalledTimes(1)
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['system-options'],
    })
    const tieredRequest = vi
      .mocked(updateSystemOption)
      .mock.calls.map(([request]) => request)
      .find(
        (request) => request.key === 'group_ratio_setting.model_tiered_ratios'
      )
    expect(JSON.parse(tieredRequest?.value as string)).toEqual({
      premium: {
        progress_basis: 'original',
        models: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'UTC',
            tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
          },
        },
      },
    })
  })

  test('keeps network-failure drafts without invalidating and retries from the failed field', async () => {
    vi.mocked(updateSystemOption)
      .mockResolvedValueOnce({ success: true, message: '' })
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ success: true, message: '' })
    const user = userEvent.setup()
    const queryClient = renderRatioSettingsCard('{}')
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
    const groupRatio = screen.getByRole('textbox', { name: 'Group ratios' })
    const topupRatio = screen.getByRole('textbox', {
      name: 'Top-up group ratios',
    })

    fireEvent.input(groupRatio, { target: { value: '{"premium":0.8}' } })
    fireEvent.input(topupRatio, { target: { value: '{"premium":1.2}' } })
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    await waitFor(() => expect(updateSystemOption).toHaveBeenCalledTimes(2))
    expect(invalidateQueries).not.toHaveBeenCalled()
    expect(groupRatio).toHaveValue('{"premium":0.8}')
    expect(topupRatio).toHaveValue('{"premium":1.2}')

    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    await waitFor(() => expect(updateSystemOption).toHaveBeenCalledTimes(3))
    expect(
      vi.mocked(updateSystemOption).mock.calls.map(([request]) => request.key)
    ).toEqual(['GroupRatio', 'TopupGroupRatio', 'TopupGroupRatio'])
    expect(invalidateQueries).toHaveBeenCalledTimes(1)
  })
})
