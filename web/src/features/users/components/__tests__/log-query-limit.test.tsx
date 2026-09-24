import { zodResolver } from '@hookform/resolvers/zod'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  userFormSchema,
  USER_FORM_DEFAULT_VALUES,
  type UserFormValues,
  transformFormDataToPayload,
} from '../../lib/user-form'
import { UserLogQueryLimit } from '../user-log-query-limit'

function LimitForm(props: {
  onSave: (value: unknown) => void
  enabled?: boolean
  initialLimit?: number
}) {
  const form = useForm<UserFormValues>({
    resolver: zodResolver(userFormSchema),
    defaultValues: {
      ...USER_FORM_DEFAULT_VALUES,
      username: 'customer',
      log_query_rate_limit: props.initialLimit ?? 0,
      log_query_rate_limit_policy: {
        enabled: props.enabled ?? true,
        default_limit: 60,
        window_seconds: 60,
        max_limit: 60000,
      },
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(transformFormDataToPayload(values, 42))
        )}
      >
        <UserLogQueryLimit />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('User log query limit', () => {
  it('saves a custom limit and restores inheritance with zero', async () => {
    const save = vi.fn()
    render(<LimitForm onSave={save} initialLimit={300} />)
    const input = screen.getByRole('spinbutton', { name: 'Log query limit' })
    expect(input).toHaveValue(300)
    expect(
      screen.getByText('Global default: 60 requests / 60 seconds')
    ).toBeInTheDocument()
    fireEvent.change(input, { target: { value: '120' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(save).toHaveBeenLastCalledWith(
        expect.objectContaining({ id: 42, log_query_rate_limit: 120 })
      )
    )
    fireEvent.click(screen.getByRole('button', { name: 'Use global default' }))
    expect(input).toHaveValue(0)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(save).toHaveBeenLastCalledWith(
        expect.objectContaining({ log_query_rate_limit: 0 })
      )
    )
  })

  it('rejects fractions, negative values and oversized limits before saving', async () => {
    const save = vi.fn()
    render(<LimitForm onSave={save} />)
    for (const value of ['1.5', '-1', '60001']) {
      fireEvent.change(screen.getByRole('spinbutton'), { target: { value } })
      const form = screen.getByRole('button', { name: 'Save' }).closest('form')
      if (!form) throw new Error('Save button must belong to the limit form')
      fireEvent.submit(form)
      await waitFor(() =>
        expect(screen.getByRole('spinbutton')).toHaveAttribute(
          'aria-invalid',
          'true'
        )
      )
      expect(save).not.toHaveBeenCalled()
    }
  })

  it('allows preparing an override while explaining that the limiter is disabled globally', () => {
    render(<LimitForm onSave={vi.fn()} enabled={false} />)
    expect(screen.getByRole('spinbutton')).toBeEnabled()
    expect(
      screen.getByText(
        'Log query limiting is disabled globally; saved overrides will apply when enabled.'
      )
    ).toBeInTheDocument()
  })
})
