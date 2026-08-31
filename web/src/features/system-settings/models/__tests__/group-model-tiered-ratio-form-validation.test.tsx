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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { GroupRatioForm } from '../group-ratio-form'

vi.mock('@/features/channels/api', () => ({
  getEnabledModels: vi.fn().mockResolvedValue({
    success: true,
    data: ['gpt-5'],
  }),
}))

vi.mock('../group-ratio-visual-editor', () => ({
  GroupRatioVisualEditor: () => <div>Group ratio editor</div>,
}))

vi.mock('../group-special-usable-editor', () => ({
  GroupSpecialUsableRulesEditor: () => <div>Special usable rules</div>,
}))

type GroupFormValues = {
  GroupRatio: string
  TopupGroupRatio: string
  UserUsableGroups: string
  GroupGroupRatio: string
  AutoGroups: string
  MaxTokenAutoGroups: number
  DefaultUseAutoGroup: boolean
  GroupSpecialUsableGroup: string
  ModelTieredRatios: string
}

const modelTieredRatios = JSON.stringify({
  premium: {
    'custom-model': {
      enabled: true,
      effective_from: 1_788_192_000,
      effective_until: null,
      timezone: 'Asia/Shanghai',
      tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
    },
  },
})

const modelTieredRatiosWithEnd = JSON.stringify({
  premium: {
    'custom-model': {
      enabled: true,
      effective_from: 1_788_192_000,
      effective_until: 1_790_870_400,
      timezone: 'Asia/Shanghai',
      tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
    },
  },
})

const activePolicyGuide =
  'Saving policy changes can still succeed, but if the current billing period already has usage, later settlement will fail with a policy hash conflict when progress basis, tiers, timezone, or end time change. Set a new effective_from (usually the change time) to open a new period; if it is in the future, the fixed GroupRatio applies until then.'

type FormHarnessProps = {
  actionsContainer: HTMLDivElement
  onSave: (values: GroupFormValues) => Promise<void>
  tieredRatios: string
  groupRatio?: string
  topupGroupRatio?: string
  userUsableGroups?: string
}

function FormHarness(props: FormHarnessProps) {
  const form = useForm<GroupFormValues>({
    defaultValues: {
      GroupRatio: props.groupRatio ?? '{"premium":1}',
      TopupGroupRatio: props.topupGroupRatio ?? '{}',
      UserUsableGroups: props.userUsableGroups ?? '{}',
      GroupGroupRatio: '{}',
      AutoGroups: '[]',
      MaxTokenAutoGroups: 5,
      DefaultUseAutoGroup: false,
      GroupSpecialUsableGroup: '{}',
      ModelTieredRatios: props.tieredRatios,
    },
  })

  return (
    <SettingsPageProvider actionsContainer={props.actionsContainer}>
      <GroupRatioForm form={form} onSave={props.onSave} isSaving={false} />
    </SettingsPageProvider>
  )
}

function renderGroupRatioForm(
  onSave = vi.fn().mockResolvedValue(undefined),
  tieredRatios = modelTieredRatios,
  groups: Pick<
    FormHarnessProps,
    'groupRatio' | 'topupGroupRatio' | 'userUsableGroups'
  > = {}
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const actionsContainer = document.createElement('div')
  actionsContainer.dataset.tieredActions = 'true'
  document.body.appendChild(actionsContainer)

  render(
    <QueryClientProvider client={queryClient}>
      <FormHarness
        actionsContainer={actionsContainer}
        onSave={onSave}
        tieredRatios={tieredRatios}
        {...groups}
      />
    </QueryClientProvider>
  )

  return onSave
}

describe('group ratio form tiered-policy draft validation', () => {
  afterEach(() => {
    document.querySelector('[data-tiered-actions]')?.remove()
  })

  test('does not submit the previous JSON while a visual group/model key is invalid', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(onSave)

    await user.clear(screen.getByLabelText('Origin model'))
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(
      screen.getAllByText('Model name is required').length
    ).toBeGreaterThan(0)
  })

  test('does not save a tiered policy whose group is absent from GroupRatio', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(
      onSave,
      modelTieredRatios.replaceAll('premium', 'orphan')
    )

    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(
      screen.getAllByText(
        'Tiered discount group "orphan" must exist in pricing groups'
      ).length
    ).toBeGreaterThan(0)
  })

  test('offers only GroupRatio keys in the tiered-policy group select', async () => {
    const user = userEvent.setup()
    renderGroupRatioForm(
      vi.fn().mockResolvedValue(undefined),
      modelTieredRatios,
      {
        topupGroupRatio: '{"topup-only":1}',
        userUsableGroups: '{"usable-only":"legacy"}',
      }
    )

    await user.click(
      screen.getByRole('combobox', {
        name: 'Billing group',
      })
    )

    expect(screen.getByRole('option', { name: 'premium' })).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'topup-only' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'usable-only' })
    ).not.toBeInTheDocument()
  })

  test('does not save while Effective start is an empty visual draft', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(onSave)

    await user.clear(screen.getByLabelText('Effective start'))
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(
      screen.getAllByText('Effective start is required').length
    ).toBeGreaterThan(0)
  })

  test('does not save while Effective end is an empty visual draft', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(onSave, modelTieredRatiosWithEnd)

    await user.clear(screen.getByLabelText('Effective end'))
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(
      screen.getAllByText('Effective end is required').length
    ).toBeGreaterThan(0)
  })

  test('does not save a charged-progress group with a zero tier ratio', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(
      onSave,
      JSON.stringify({
        premium: {
          progress_basis: 'charged',
          models: {
            'custom-model': {
              enabled: true,
              effective_from: 1_788_192_000,
              effective_until: null,
              timezone: 'Asia/Shanghai',
              tiers: [{ min_monthly_original_quota: 0, ratio: 0 }],
            },
          },
        },
      })
    )

    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Discount ratio 1')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(
      screen.getAllByText('Charged-progress tier ratios must be greater than 0')
        .length
    ).toBeGreaterThan(0)
  })

  test('does not turn a cleared original-progress ratio into a free tier', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(onSave)

    const ratio = screen.getByLabelText('Discount ratio 1')
    await user.clear(ratio)
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(ratio).toHaveValue(null)
    expect(ratio).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getAllByText('Please enter a valid number').length
    ).toBeGreaterThan(0)
  })

  test('keeps the settlement-risk warning visible in visual and JSON modes', async () => {
    const user = userEvent.setup()
    renderGroupRatioForm()

    expect(screen.getByText(activePolicyGuide)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))

    expect(screen.getByText(activePolicyGuide)).toBeInTheDocument()
  })

  test('can save corrected JSON after the visual editor reported an invalid configuration', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderGroupRatioForm(onSave, '{"premium":')

    expect(
      await screen.findByText(
        'Tiered discount JSON cannot be shown visually. Switch to JSON mode to fix it.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))

    const textarea = document.querySelector<HTMLTextAreaElement>(
      'textarea[name="ModelTieredRatios"]'
    )
    if (!textarea) throw new Error('tiered-discount JSON editor is missing')
    fireEvent.input(textarea, { target: { value: '{}' } })
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  })

  test('explains how to safely open a new period when changing an active policy', async () => {
    const user = userEvent.setup()
    renderGroupRatioForm()

    await user.click(screen.getByRole('button', { name: 'Usage guide' }))

    const pricingSection = screen
      .getByRole('heading', { name: 'How a call is priced' })
      .closest('section')
    expect(pricingSection).not.toBeNull()
    expect(
      within(pricingSection as HTMLElement).getByText(activePolicyGuide)
    ).toBeInTheDocument()
    expect(
      within(pricingSection as HTMLElement).getByText(
        "With a monthly tiered policy, the group's progress-basis switch chooses whether thresholds advance by the original model price or the high-precision settled amount after the tier discount. Requests crossing a threshold are split between tiers. Without a matching policy, cost = model price × fixed ratio."
      )
    ).toBeInTheDocument()
  })
})
