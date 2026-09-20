import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'

import type { VideoMediaCapability } from '../../../types'
import { VideoMediaCapabilityFields } from '../video-media-capability-fields'

function VideoCapabilityForm() {
  const [media, setMedia] = useState<VideoMediaCapability>()
  return (
    <>
      <VideoMediaCapabilityFields
        path='messages.*.content.*.video_url'
        value={media}
        onChange={setMedia}
      />
      <output aria-label='Saved media capability'>
        {JSON.stringify(media ?? null)}
      </output>
    </>
  )
}

describe('video delivery controls', () => {
  it('configures separate byte limits, supports explicit disabling, and resets the override', async () => {
    const user = userEvent.setup()
    render(<VideoCapabilityForm />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure video delivery' })
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'URL maximum bytes' }),
      { target: { value: '524288000' } }
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Base64 maximum bytes' }),
      { target: { value: '20971520' } }
    )
    await user.click(screen.getByRole('combobox', { name: 'Accept video URL' }))
    await user.click(
      within(await screen.findByRole('listbox')).getByRole('option', {
        name: 'Enabled',
      })
    )
    expect(
      screen.getByRole('combobox', { name: 'Accept video URL' })
    ).toHaveTextContent('Enabled')
    await user.click(
      screen.getByRole('combobox', { name: 'Accept video Base64' })
    )
    await user.click(
      within(await screen.findByRole('listbox')).getByRole('option', {
        name: 'Disabled',
      })
    )
    expect(
      JSON.parse(
        screen.getByLabelText('Saved media capability').textContent ?? ''
      )
    ).toEqual({
      kind: 'video',
      formats: {
        url: { supported: true, max_media_bytes: 524288000 },
        base64: { supported: false, max_media_bytes: 20971520 },
      },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Remove video delivery override' })
    )
    expect(screen.getByLabelText('Saved media capability')).toHaveTextContent(
      'null'
    )
    expect(
      screen.queryByRole('spinbutton', { name: 'URL maximum bytes' })
    ).not.toBeInTheDocument()
  })
})
