import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { UpstreamTransportDetails } from '../upstream-transport-details'

describe('upstream transport diagnostics', () => {
  it('distinguishes unknown body size and absent timings from zero', () => {
    render(
      <UpstreamTransportDetails
        attempts={[
          {
            attempt: 3,
            channel_id: 15,
            body_bytes: -1,
            reason: 'unknown_length',
            policy: 'auto',
            protocol: 'HTTP/2.0',
            reused: true,
            connection_acquisitions: 1,
            write_callbacks: 1,
            outcome: 'complete',
            write_ms: 0,
          },
        ]}
      />
    )
    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(
      screen.getByText('Request body bytes').nextElementSibling
    ).toHaveTextContent('Unknown')
    expect(
      screen.getByText('Connection acquisition (ms)').nextElementSibling
    ).toHaveTextContent('—')
    expect(
      screen.getByText('Write after acquisition (ms)').nextElementSibling
    ).toHaveTextContent('0.00')
    expect(screen.getByText(/#3/)).toHaveTextContent('HTTP/2.0')
    expect(screen.getByText(/unknown_length/)).toHaveTextContent('auto')
  })
  it('does not render a diagnostic section for legacy logs', () => {
    const { container } = render(<UpstreamTransportDetails attempts={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
