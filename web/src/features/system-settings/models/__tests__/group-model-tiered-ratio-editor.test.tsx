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
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getEnabledModels } from '@/features/channels/api'

import { GroupModelTieredRatioEditor } from '../group-model-tiered-ratio-editor'

vi.mock('@/features/channels/api', () => ({
  getEnabledModels: vi.fn(),
}))

const configuredValue = JSON.stringify({
  premium: {
    'custom-model': {
      enabled: true,
      effective_from: 1_788_192_000,
      effective_until: null,
      timezone: 'Asia/Shanghai',
      tiers: [
        { min_monthly_original_quota: 0, ratio: 1 },
        { min_monthly_original_quota: 500_000, ratio: 0.9 },
      ],
    },
  },
})

const minimalPolicyJson = JSON.stringify({
  enabled: false,
  effective_from: 1_788_192_000,
  effective_until: null,
  timezone: 'Asia/Shanghai',
  tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
})

const activePolicyChangeWarning =
  "If this active policy's current period has usage, changing progress basis, tiers, timezone, or end time without a new effective_from can make later settlement fail with a policy hash conflict."

type RenderEditorOptions = {
  initialValue?: string
  now?: number
  groupOptions?: string[]
}

type CanonicalConfig<T> = Record<
  string,
  { progress_basis: string; models: Record<string, T> }
>

function renderEditor(
  onChange = vi.fn(),
  onValidationChange = vi.fn(),
  options: RenderEditorOptions = {}
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  function EditorHarness() {
    const [value, setValue] = useState(options.initialValue ?? configuredValue)

    return (
      <GroupModelTieredRatioEditor
        value={value}
        groupOptions={options.groupOptions ?? ['default', 'premium']}
        onChange={(nextValue) => {
          setValue(nextValue)
          onChange(nextValue)
        }}
        onValidationChange={onValidationChange}
        now={options.now ?? 1_788_624_000}
      />
    )
  }

  render(
    <QueryClientProvider client={queryClient}>
      <EditorHarness />
    </QueryClientProvider>
  )

  return onChange
}

describe('group-model tiered ratio editor', () => {
  beforeEach(() => {
    vi.mocked(getEnabledModels).mockResolvedValue({
      success: true,
      data: ['gpt-5', 'claude-sonnet-4'],
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  test('shows one accessible progress-basis switch per group and normalizes legacy JSON on toggle', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    const progressSwitch = screen.getByRole('switch', {
      name: 'Advance premium tiers by settled quota',
    })

    expect(progressSwitch).not.toBeChecked()
    expect(
      screen.getAllByRole('switch', { name: /tiers by settled quota/ })
    ).toHaveLength(1)
    expect(
      screen.getByText(
        'Thresholds use monthly original quota. A request crossing a threshold is split between tiers.'
      )
    ).toBeInTheDocument()

    await user.click(progressSwitch)

    expect(progressSwitch).toBeChecked()
    expect(
      screen.getByText(
        'Thresholds use monthly settled quota. At 80%, an original price of 20 advances 16; requests crossing a threshold are still split between tiers.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByLabelText('Monthly settled quota threshold 2')
    ).toBeInTheDocument()
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as Record<
      string,
      { progress_basis: string; models: Record<string, unknown> }
    >
    expect(result.premium.progress_basis).toBe('charged')
    expect(result.premium.models['custom-model']).toBeDefined()
  })

  test('changes the progress basis for only the selected group', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          progress_basis: 'original',
          models: { 'gpt-5': JSON.parse(minimalPolicyJson) },
        },
        default: {
          progress_basis: 'original',
          models: { 'claude-sonnet-4': JSON.parse(minimalPolicyJson) },
        },
      }),
    })

    await user.click(
      screen.getByRole('switch', {
        name: 'Advance premium tiers by settled quota',
      })
    )

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as Record<string, { progress_basis: string }>
    expect(result.premium.progress_basis).toBe('charged')
    expect(result.default.progress_basis).toBe('original')
    expect(
      screen.getByRole('switch', {
        name: 'Advance default tiers by settled quota',
      })
    ).not.toBeChecked()
  })

  test('marks the zero ratio field invalid after charged progress is enabled', async () => {
    const user = userEvent.setup()
    renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            ...JSON.parse(minimalPolicyJson),
            tiers: [{ min_monthly_original_quota: 0, ratio: 0 }],
          },
        },
      }),
    })

    await user.click(
      screen.getByRole('switch', {
        name: 'Advance premium tiers by settled quota',
      })
    )

    expect(screen.getByLabelText('Discount ratio 1')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(
      screen.getByText('Charged-progress tier ratios must be greater than 0')
    ).toBeInTheDocument()
    const ratio = screen.getByLabelText('Discount ratio 1')
    const ratioErrorId = ratio.getAttribute('aria-describedby')
    expect(ratioErrorId).toBeTruthy()
    expect(document.querySelector(`#${ratioErrorId}`)).toHaveTextContent(
      'Charged-progress tier ratios must be greater than 0'
    )
  })

  test('shows activation metadata, active status, and a custom-model warning', async () => {
    renderEditor()

    expect(
      screen.getByRole('heading', { name: 'Monthly tiered discounts' })
    ).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText(activePolicyChangeWarning)).toBeInTheDocument()
    expect(screen.getByLabelText('Effective start')).toHaveAttribute(
      'type',
      'datetime-local'
    )
    expect(screen.getByLabelText('Effective start')).toHaveValue(
      '2026-09-01T00:00'
    )
    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Shanghai')
    expect(screen.getByText('No end time')).toBeInTheDocument()
    expect(
      await screen.findByText(
        'This model is not currently enabled. The policy will still be saved.'
      )
    ).toBeInTheDocument()
  })

  test('hides the active-policy change warning before the effective start', () => {
    renderEditor(vi.fn(), vi.fn(), { now: 1_788_105_600 })

    expect(screen.getByText('Scheduled')).toBeInTheDocument()
    expect(
      screen.queryByText(activePolicyChangeWarning)
    ).not.toBeInTheDocument()
  })

  test('converts an activation time using the selected policy timezone', async () => {
    const onChange = renderEditor()

    fireEvent.change(screen.getByLabelText('Effective start'), {
      target: { value: '2026-09-02T00:00' },
    })

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ effective_from: number }>
    expect(result.premium.models['custom-model'].effective_from).toBe(
      1_788_278_400
    )
  })

  test('rejects a nonexistent local time during the New York DST gap', () => {
    const onChange = vi.fn()
    const onValidationChange = vi.fn()
    renderEditor(onChange, onValidationChange, {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: Date.UTC(2026, 2, 7, 17) / 1000,
            effective_until: null,
            timezone: 'America/New_York',
            tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
          },
        },
      }),
    })
    const startInput = screen.getByLabelText('Effective start')

    fireEvent.change(startInput, {
      target: { value: '2026-03-08T02:30' },
    })

    expect(startInput).toHaveValue('2026-03-08T02:30')
    expect(startInput).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getByText('Activation time must be a Unix timestamp')
    ).toBeInTheDocument()
    expect(onValidationChange).toHaveBeenLastCalledWith(
      'Activation time must be a Unix timestamp'
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  test.each([
    ['America/New_York', '2026-11-01T01:30'],
    ['Europe/Berlin', '2026-10-25T02:30'],
  ])('rejects an ambiguous local time in %s', (timezone, localTime) => {
    const onChange = vi.fn()
    const onValidationChange = vi.fn()
    renderEditor(onChange, onValidationChange, {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone,
            tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
          },
        },
      }),
    })
    const startInput = screen.getByLabelText('Effective start')

    fireEvent.change(startInput, { target: { value: localTime } })

    expect(startInput).toHaveValue(localTime)
    expect(startInput).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getByText('Activation time must be a Unix timestamp')
    ).toBeInTheDocument()
    expect(onValidationChange).toHaveBeenLastCalledWith(
      'Activation time must be a Unix timestamp'
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  test('accepts an unambiguous New York local time after the DST overlap', async () => {
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'America/New_York',
            tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
          },
        },
      }),
    })

    fireEvent.change(screen.getByLabelText('Effective start'), {
      target: { value: '2026-11-02T01:30' },
    })

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ effective_from: number }>
    expect(result.premium.models['gpt-5'].effective_from).toBe(1_793_601_000)
  })

  test('keeps a cleared effective start as an invalid draft instead of epoch zero', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const onValidationChange = vi.fn()
    renderEditor(onChange, onValidationChange)
    const startInput = screen.getByLabelText('Effective start')

    await user.clear(startInput)

    expect(startInput).toHaveValue('')
    expect(startInput).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('Effective start is required')).toBeInTheDocument()
    const startErrorId = startInput.getAttribute('aria-describedby')
    expect(startErrorId).toBeTruthy()
    expect(document.querySelector(`#${startErrorId}`)).toHaveTextContent(
      'Effective start is required'
    )
    expect(onValidationChange).toHaveBeenLastCalledWith(
      'Effective start is required'
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  test('keeps a cleared effective end as an invalid draft instead of epoch zero', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const onValidationChange = vi.fn()
    const valueWithEnd = JSON.stringify({
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
    renderEditor(onChange, onValidationChange, {
      initialValue: valueWithEnd,
    })
    const endInput = screen.getByLabelText('Effective end')

    await user.clear(endInput)

    expect(endInput).toHaveValue('')
    expect(endInput).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('Effective end is required')).toBeInTheDocument()
    const endErrorId = endInput.getAttribute('aria-describedby')
    expect(endErrorId).toBeTruthy()
    expect(document.querySelector(`#${endErrorId}`)).toHaveTextContent(
      'Effective end is required'
    )
    expect(onValidationChange).toHaveBeenLastCalledWith(
      'Effective end is required'
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  test('adds and removes a tier while preserving the JSON contract', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()

    await user.click(screen.getByRole('button', { name: 'Add tier' }))

    expect(
      screen.getByLabelText('Monthly original quota threshold 3')
    ).toBeInTheDocument()
    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const addedValue = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ tiers: unknown[] }>
    expect(addedValue.premium.models['custom-model'].tiers).toHaveLength(3)

    const thirdTier = screen
      .getByText('Tier 3')
      .closest('[data-tier-row="true"]')
    expect(thirdTier).not.toBeNull()
    await user.click(
      within(thirdTier as HTMLElement).getByRole('button', {
        name: 'Remove tier 3',
      })
    )

    const removedValue = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ tiers: unknown[] }>
    expect(removedValue.premium.models['custom-model'].tiers).toHaveLength(2)
  })

  test('adds a zero-threshold full-price tier when a policy has no tiers', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'Asia/Shanghai',
            tiers: [],
          },
        },
      }),
    })

    await user.click(screen.getByRole('button', { name: 'Add tier' }))

    expect(
      screen.getByLabelText('Monthly original quota threshold 1')
    ).toHaveValue(0)
    expect(screen.getByLabelText('Discount ratio 1')).toHaveValue(1)
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ tiers: unknown[] }>
    expect(result.premium.models['gpt-5'].tiers).toEqual([
      { min_monthly_original_quota: 0, ratio: 1 },
    ])
  })

  test('keeps an invalid first-tier threshold editable so it can be repaired to zero', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'Asia/Shanghai',
            tiers: [{ min_monthly_original_quota: 10, ratio: 1 }],
          },
        },
      }),
    })
    const threshold = screen.getByLabelText(
      'Monthly original quota threshold 1'
    )

    expect(threshold).not.toBeDisabled()
    await user.clear(threshold)
    await user.type(threshold, '0')

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{
      tiers: Array<{ min_monthly_original_quota: number }>
    }>
    expect(
      result.premium.models['gpt-5'].tiers[0].min_monthly_original_quota
    ).toBe(0)
  })

  test('adds a disabled policy for the first enabled model without creating a wildcard', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    await screen.findByText(
      'This model is not currently enabled. The policy will still be saved.'
    )

    await user.click(screen.getByRole('button', { name: 'Add policy' }))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ enabled: boolean }>
    expect(result.default.models['gpt-5']).toMatchObject({ enabled: false })
    expect(result.default.models['*']).toBeUndefined()
  })

  test('adds policies for prototype-named groups and models without reading inherited members', async () => {
    const user = userEvent.setup()
    const groupOnChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: '{}',
      groupOptions: ['constructor'],
    })

    await user.click(screen.getByRole('button', { name: 'Add policy' }))

    const groupResult = JSON.parse(
      groupOnChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(Object.hasOwn(groupResult, 'constructor')).toBe(true)
    expect(groupResult['constructor'].models['gpt-5']).toBeDefined()

    vi.mocked(getEnabledModels).mockResolvedValue({
      success: true,
      data: ['toString'],
    })
    const modelOnChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: '{}',
      groupOptions: ['default'],
    })
    const addButtons = screen.getAllByRole('button', { name: 'Add policy' })
    const modelAddButton = addButtons.at(-1)
    if (!modelAddButton) throw new Error('model add-policy button is missing')
    await user.click(modelAddButton)

    const modelResult = JSON.parse(
      modelOnChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(Object.hasOwn(modelResult.default.models, 'toString')).toBe(true)
  })

  test('adds a model without changing the existing group progress basis', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        default: {
          progress_basis: 'charged',
          models: { 'gpt-5': JSON.parse(minimalPolicyJson) },
        },
      }),
    })

    await user.click(screen.getByRole('button', { name: 'Add policy' }))

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.default.progress_basis).toBe('charged')
    expect(result.default.models['gpt-5']).toBeDefined()
    expect(result.default.models['claude-sonnet-4']).toBeDefined()
  })

  test('uses the next custom model name when no enabled model is available', async () => {
    vi.mocked(getEnabledModels).mockResolvedValue({ success: true, data: [] })
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        default: {
          'custom-model-1': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'Asia/Shanghai',
            tiers: [{ min_monthly_original_quota: 0, ratio: 1 }],
          },
        },
      }),
    })
    await screen.findByText(
      'This model is not currently enabled. The policy will still be saved.'
    )

    await user.click(screen.getByRole('button', { name: 'Add policy' }))

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{ enabled: boolean }>
    expect(result.default.models['custom-model-2']).toMatchObject({
      enabled: false,
    })
    expect(result.default.models['*']).toBeUndefined()
  })

  test('skips a wildcard returned by enabled models and selects the next exact model', async () => {
    vi.mocked(getEnabledModels).mockResolvedValue({
      success: true,
      data: ['*', 'gpt-5'],
    })
    const user = userEvent.setup()
    const onChange = renderEditor()
    await screen.findByText(
      'This model is not currently enabled. The policy will still be saved.'
    )

    await user.click(screen.getByRole('button', { name: 'Add policy' }))

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.default.models['gpt-5']).toBeDefined()
    expect(result.default.models['*']).toBeUndefined()
  })

  test('moves a policy only through the configured pricing-group select', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    const groupSelect = screen.getByRole('combobox', {
      name: 'Billing group',
    })

    await user.click(groupSelect)
    await user.click(screen.getByRole('option', { name: 'default' }))

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.default.models['custom-model']).toBeDefined()
    expect(result.premium).toBeUndefined()
  })

  test('moves a policy through the pricing-group select with the keyboard', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    const groupSelect = screen.getByRole('combobox', {
      name: 'Billing group',
    })

    groupSelect.focus()
    await user.keyboard('{Enter}{Home}{Enter}')

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.default.models['custom-model']).toBeDefined()
    expect(result.premium).toBeUndefined()
  })

  test('marks an existing policy invalid when its group is not a pricing group', async () => {
    const user = userEvent.setup()
    const onValidationChange = vi.fn()
    renderEditor(vi.fn(), onValidationChange, {
      groupOptions: ['default'],
    })

    const groupSelect = screen.getByRole('combobox', {
      name: 'Billing group',
    })
    expect(groupSelect).toHaveAttribute('aria-invalid', 'true')
    await user.click(groupSelect)
    expect(screen.getByRole('option', { name: 'premium' })).toHaveAttribute(
      'aria-disabled',
      'true'
    )
    expect(
      screen.getByText(
        'Tiered discount group "premium" must exist in pricing groups'
      )
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(onValidationChange).toHaveBeenLastCalledWith(
        'Tiered discount group "premium" must exist in pricing groups'
      )
    )
  })

  test('marks only an overlong model draft invalid and links its error message', () => {
    renderEditor()
    const groupInput = screen.getByLabelText('Billing group')
    const modelInput = screen.getByLabelText('Origin model')
    const modelAtLimit = '模'.repeat(85)

    fireEvent.change(modelInput, { target: { value: modelAtLimit } })
    expect(modelInput).toHaveAttribute('aria-invalid', 'false')

    fireEvent.change(modelInput, { target: { value: `${modelAtLimit}a` } })
    expect(modelInput).toHaveAttribute('aria-invalid', 'true')
    expect(groupInput).toHaveAttribute('aria-invalid', 'false')
    const errorId = modelInput.getAttribute('aria-describedby')
    expect(errorId).toBeTruthy()
    expect(document.querySelector(`#${errorId}`)).toHaveTextContent(
      'Model name must be at most 255 UTF-8 bytes'
    )
  })

  test('rejects an exact __proto__ model draft before it can be serialized', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    renderEditor(onChange)
    const input = screen.getByLabelText('Origin model')

    await user.clear(input)
    await user.type(input, '__proto__')
    await user.tab()

    expect(input).toHaveAttribute('aria-invalid', 'true')
    const errorId = input.getAttribute('aria-describedby')
    expect(errorId).toBeTruthy()
    expect(document.querySelector(`#${errorId}`)).toHaveTextContent(
      'Group and model names cannot use __proto__'
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  test('marks an invalid threshold and exposes the validation message', async () => {
    const user = userEvent.setup()
    renderEditor()
    const threshold = screen.getByLabelText(
      'Monthly original quota threshold 2'
    )

    await user.clear(threshold)
    await user.type(threshold, '0')

    expect(threshold).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getByText('Tier thresholds must be strictly increasing')
    ).toBeInTheDocument()
    const thresholdErrorId = threshold.getAttribute('aria-describedby')
    expect(thresholdErrorId).toBeTruthy()
    expect(document.querySelector(`#${thresholdErrorId}`)).toHaveTextContent(
      'Tier thresholds must be strictly increasing'
    )
  })

  test('renders a timezone validation error beside and linked to its field', () => {
    renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            ...JSON.parse(minimalPolicyJson),
            timezone: 'Mars/Olympus_Mons',
          },
        },
      }),
    })
    const timezone = screen.getByLabelText('Timezone')

    expect(timezone).toHaveAttribute('aria-invalid', 'true')
    const timezoneErrorId = timezone.getAttribute('aria-describedby')
    expect(timezoneErrorId).toBeTruthy()
    const timezoneError = document.querySelector<HTMLElement>(
      `#${timezoneErrorId}`
    )
    expect(timezoneError).toHaveTextContent('Timezone is invalid')
    expect(timezone.parentElement).toContainElement(timezoneError)
  })

  test('updates a ratio without changing unrelated policy fields', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    const ratio = screen.getByLabelText('Discount ratio 2')

    await user.clear(ratio)
    await user.type(ratio, '0.8')

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<{
      enabled: boolean
      effective_from: number
      effective_until: number | null
      timezone: string
      tiers: Array<{
        min_monthly_original_quota: number
        ratio: number
      }>
    }>
    const policy = result.premium.models['custom-model']

    expect(policy.enabled).toBe(true)
    expect(policy.effective_from).toBe(1_788_192_000)
    expect(policy.effective_until).toBeNull()
    expect(policy.timezone).toBe('Asia/Shanghai')
    expect(policy.tiers[1]).toEqual({
      min_monthly_original_quota: 500_000,
      ratio: 0.8,
    })
  })

  test('commits a complete custom model name after editing without losing focus', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor()
    const modelInput = screen.getByLabelText('Origin model')

    await user.clear(modelInput)
    await user.type(modelInput, 'new-custom-model')
    expect(modelInput).toHaveFocus()
    await user.tab()

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.premium.models['new-custom-model']).toBeDefined()
    expect(result.premium.models['custom-model']).toBeUndefined()
  })

  test('renames a model without changing its group wrapper or progress basis', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          progress_basis: 'charged',
          models: { 'gpt-5': JSON.parse(minimalPolicyJson) },
        },
      }),
    })

    const modelInput = screen.getByLabelText('Origin model')
    await user.clear(modelInput)
    await user.type(modelInput, 'renamed-model')
    await user.tab()

    const result = JSON.parse(
      onChange.mock.calls.at(-1)?.[0] as string
    ) as CanonicalConfig<unknown>
    expect(result.premium.progress_basis).toBe('charged')
    expect(result.premium.models['renamed-model']).toBeDefined()
    expect(result.premium.models['gpt-5']).toBeUndefined()
  })

  test('deletes the group wrapper when its last model is removed', async () => {
    const user = userEvent.setup()
    const onChange = renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          progress_basis: 'charged',
          models: { 'gpt-5': JSON.parse(minimalPolicyJson) },
        },
      }),
    })

    await user.click(
      screen.getByRole('button', { name: 'Remove tiered discount policy' })
    )

    expect(JSON.parse(onChange.mock.calls.at(-1)?.[0] as string)).toEqual({})
  })

  test('reports an invalid key draft so the parent form can block saving', async () => {
    const user = userEvent.setup()
    const onValidationChange = vi.fn()
    renderEditor(vi.fn(), onValidationChange)

    await user.clear(screen.getByLabelText('Origin model'))

    expect(screen.getByLabelText('Origin model')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(onValidationChange).toHaveBeenLastCalledWith(
      'Model name is required'
    )
  })

  test.each([
    [
      'duplicate group keys',
      `{"premium":{"gpt-5":${minimalPolicyJson}},"premium":{"claude-sonnet-4":${minimalPolicyJson}}}`,
    ],
    [
      'duplicate model keys',
      `{"premium":{"gpt-5":${minimalPolicyJson},"gpt-5":${minimalPolicyJson}}}`,
    ],
    [
      'a null tier',
      '{"premium":{"gpt-5":{"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"Asia/Shanghai","tiers":[null]}}}',
    ],
    [
      'a tier without renderable fields',
      '{"premium":{"gpt-5":{"enabled":false,"effective_from":1788192000,"effective_until":null,"timezone":"Asia/Shanghai","tiers":[{}]}}}',
    ],
  ])('falls back to JSON repair mode for %s', (_label, initialValue) => {
    expect(() => renderEditor(vi.fn(), vi.fn(), { initialValue })).not.toThrow()
    expect(
      screen.getByText(
        'Tiered discount JSON cannot be shown visually. Switch to JSON mode to fix it.'
      )
    ).toBeInTheDocument()
  })

  test('keeps render-safe business validation errors editable in visual mode', () => {
    renderEditor(vi.fn(), vi.fn(), {
      initialValue: JSON.stringify({
        premium: {
          'gpt-5': {
            enabled: false,
            effective_from: 1_788_192_000,
            effective_until: null,
            timezone: 'Asia/Shanghai',
            tiers: [{ min_monthly_original_quota: 0, ratio: 1.2 }],
          },
        },
      }),
    })

    expect(screen.getByLabelText('Discount ratio 1')).toHaveValue(1.2)
    expect(screen.getByLabelText('Discount ratio 1')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(
      screen.getByText('Ratio must be a finite number between 0 and 1')
    ).toBeInTheDocument()
  })
})
