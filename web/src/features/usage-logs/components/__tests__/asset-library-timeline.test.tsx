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

// @vitest-environment happy-dom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { AssetLibraryTimeline } from '../asset-library-timeline'
import { DetailsDialog } from '../dialogs/details-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(cleanup)

describe('asset library timeline', () => {
  it('shows the timeline inside admin log details and hides it from ordinary users', async () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1,
      type: 8,
      content: 'Asset request',
      other: JSON.stringify({
        admin_info: {
          asset_timing: {
            version: 1,
            request_id: 'admin-only-id',
            action: 'CreateAsset',
            duration_ms: 100,
            started_at_ms: 1000,
            completed_at_ms: 1100,
            outcome: 'succeeded',
            stages: [
              {
                id: 1,
                stage: 'source_download',
                started_at_ms: 1000,
                duration_ms: 100,
                outcome: 'succeeded',
              },
            ],
          },
        },
      }),
    })
    const result = render(
      <DetailsDialog log={log} isAdmin open onOpenChange={() => {}} />
    )
    expect(
      await screen.findByRole('region', { name: 'Request Timeline' })
    ).toBeTruthy()
    result.rerender(
      <DetailsDialog log={log} isAdmin={false} open onOpenChange={() => {}} />
    )
    expect(
      screen.queryByRole('region', { name: 'Request Timeline' })
    ).toBeNull()
    expect(screen.queryByText('admin-only-id')).toBeNull()
  })
  it('highlights lock waiting and displays failed upstream diagnostics separately', async () => {
    render(
      <AssetLibraryTimeline
        timing={{
          version: 1,
          request_id: 'req-1',
          action: 'CreateAsset',
          duration_ms: 1200,
          started_at_ms: 1000,
          completed_at_ms: 2200,
          outcome: 'failed',
          stages: [
            {
              stage: 'channel_lock',
              channel_id: 7,
              started_at_ms: 1000,
              duration_ms: 1000,
              outcome: 'succeeded',
            },
            {
              stage: 'upstream_request',
              channel_id: 7,
              operation: 'CreateAsset',
              started_at_ms: 2000,
              duration_ms: 200,
              outcome: 'failed',
              http_status: 503,
              upstream_request_id: 'up-1',
            },
          ],
        }}
      />
    )
    expect(
      screen.getByRole('region', { name: 'Request Timeline' })
    ).toBeTruthy()
    expect(screen.getByText(/Longest Stage/)).toBeTruthy()
    expect(screen.getAllByText('Channel Lock Wait')[0]).toBeTruthy()
    await userEvent.click(screen.getByText('Stage Details'))
    expect(screen.getByText('up-1')).toBeTruthy()
    expect(screen.getByText('503')).toBeTruthy()
  })

  it('keeps cross-request activation waiting separate from request duration', () => {
    render(
      <AssetLibraryTimeline
        timing={{
          version: 1,
          request_id: 'req-2',
          action: 'GetAsset',
          duration_ms: 50,
          started_at_ms: 9000,
          completed_at_ms: 9050,
          outcome: 'succeeded',
          stages: [],
          replicas: [
            {
              asset_id: 'asset-na-1',
              channel_id: 7,
              status: 'ready',
              upload_started_at_ms: 1000,
              submitted_at_ms: 2000,
              first_active_at_ms: 9000,
              last_polled_at_ms: 9050,
              poll_count: 3,
            },
          ],
        }}
      />
    )
    expect(screen.getByText('First Active Observation')).toBeTruthy()
    expect(
      screen.getByText(
        /Includes polling intervals; not pure upstream processing time/
      )
    ).toBeTruthy()
    expect(screen.getByText('7.0s')).toBeTruthy()
    expect(screen.getByText('50 ms')).toBeTruthy()
  })

  it('does not invent stages for legacy or malformed timing', () => {
    render(
      <AssetLibraryTimeline
        timing={{
          version: 1,
          request_id: 'old',
          action: 'GetAsset',
          duration_ms: 0,
          started_at_ms: 0,
          completed_at_ms: 0,
          outcome: 'succeeded',
          stages: [],
        }}
      />
    )
    expect(screen.queryByText('Channel Lock Wait')).toBeNull()
    expect(screen.queryByText('First Active Observation')).toBeNull()
  })
})
