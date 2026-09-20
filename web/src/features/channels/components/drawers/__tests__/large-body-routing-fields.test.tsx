import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, it } from 'vitest'
import { Form } from '@/components/ui/form'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../lib/channel-form'
import { LargeBodyRoutingFields } from '../large-body-routing-fields'

function Fixture({ protocol = 'auto' }: { protocol?: 'auto' | 'http1' }) {
  const form = useForm<ChannelFormValues>({
    defaultValues: { ...CHANNEL_FORM_DEFAULT_VALUES, http_protocol: protocol },
  })
  return (
    <Form {...form}>
      <LargeBodyRoutingFields control={form.control} />
    </Form>
  )
}

describe('large-body routing controls', () => {
  it('reveals an editable KiB threshold only when enabled', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('switch', {
        name: 'Use HTTP/1.1 for large request bodies',
      })
    )
    const threshold = screen.getByRole('spinbutton', {
      name: 'Request body threshold (KiB)',
    })
    expect(threshold).toHaveValue(1024)
    fireEvent.change(threshold, { target: { value: '2048' } })
    expect(threshold).toHaveValue(2048)
    await user.click(screen.getByRole('switch'))
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
  })
  it('disables redundant routing in forced HTTP/1.1 mode', () => {
    render(<Fixture protocol='http1' />)
    expect(screen.getByRole('switch')).toHaveAttribute('aria-disabled', 'true')
  })
})
