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
import { expect, test } from 'vitest'

import { detachPoolChannel } from '../priority-groups'

test.each([
  {
    name: 'between adjacent groups',
    priorities: [13, 12],
    position: 1,
    expected: [
      [2, 14],
      [1, 13],
      [3, 12],
    ],
  },
  {
    name: 'above the maximum priority',
    priorities: [2147483647, 12],
    position: 0,
    expected: [
      [1, 2147483647],
      [2, 2147483646],
      [3, 12],
    ],
  },
  {
    name: 'below priority zero',
    priorities: [13, 0],
    position: 2,
    expected: [
      [2, 13],
      [3, 1],
      [1, 0],
    ],
  },
  {
    name: 'in an existing numeric gap',
    priorities: [100, 80],
    position: 1,
    expected: [
      [2, 100],
      [1, 99],
      [3, 80],
    ],
  },
])(
  'detaching a channel $name preserves the remaining ties and weights',
  ({ priorities, position, expected }) => {
    const groups = [
      {
        priority: priorities[0],
        channels: [
          { channel_id: 1, priority: priorities[0], weight: 30, enabled: true },
          {
            channel_id: 2,
            priority: priorities[0],
            weight: 10,
            enabled: false,
          },
        ],
      },
      {
        priority: priorities[1],
        channels: [
          { channel_id: 3, priority: priorities[1], weight: 0, enabled: true },
        ],
      },
    ]
    const result = detachPoolChannel(groups, 1, position)
    expect(result?.map((entry) => [entry.channel_id, entry.priority])).toEqual(
      expected
    )
    expect(result?.find((entry) => entry.channel_id === 1)).toMatchObject({
      weight: 30,
      enabled: true,
    })
    expect(result?.find((entry) => entry.channel_id === 2)).toMatchObject({
      weight: 10,
      enabled: false,
    })
    expect(result?.find((entry) => entry.channel_id === 3)).toMatchObject({
      weight: 0,
      enabled: true,
    })
    expect(groups[0].channels[0].priority).toBe(priorities[0])
  }
)

test('dropping a standalone channel beside its original group leaves the configuration unchanged', () => {
  expect(
    detachPoolChannel(
      [
        {
          priority: 13,
          channels: [
            { channel_id: 1, priority: 13, weight: 10, enabled: true },
          ],
        },
        {
          priority: 12,
          channels: [
            { channel_id: 2, priority: 12, weight: 20, enabled: true },
          ],
        },
      ],
      1,
      1
    )
  ).toBeNull()
})

test('detaching preserves inherited priorities in groups that do not need to shift', () => {
  const inherited = {
    channel_id: 3,
    priority: null,
    weight: null,
    enabled: true,
  }
  const result = detachPoolChannel(
    [
      {
        priority: 13,
        channels: [
          { channel_id: 1, priority: 13, weight: 10, enabled: true },
          { channel_id: 2, priority: 13, weight: 20, enabled: true },
        ],
      },
      { priority: 8, channels: [inherited] },
    ],
    1,
    1
  )
  expect(result?.at(-1)).toBe(inherited)
})
