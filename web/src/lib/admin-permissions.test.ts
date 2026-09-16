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
import { describe, expect, test } from 'vitest'

import type { AuthUser } from '@/stores/auth-store'

import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
  normalizeAdminPermissions,
  type PermissionCatalog,
} from './admin-permissions'

const catalog: PermissionCatalog = {
  resources: [
    {
      resource: ADMIN_PERMISSION_RESOURCES.USER,
      label_key: 'Managed User Access',
      actions: [
        {
          action: ADMIN_PERMISSION_ACTIONS.READ,
          label_key: 'View managed users',
          description_key: 'View users that are assigned to you.',
        },
      ],
    },
  ],
  roles: [
    {
      key: 'admin',
      name: 'Administrator',
      built_in: true,
      superuser: false,
      grants: {
        [ADMIN_PERMISSION_RESOURCES.USER]: {
          [ADMIN_PERMISSION_ACTIONS.READ]: true,
        },
      },
    },
  ],
}

describe('managed user capabilities', () => {
  test('ordinary users receive only their explicit capabilities', () => {
    const user = {
      id: 7,
      username: 'manager',
      role: 1,
      permissions: {
        admin_permissions: {
          [ADMIN_PERMISSION_RESOURCES.USER]: {
            [ADMIN_PERMISSION_ACTIONS.READ]: true,
          },
        },
      },
    } satisfies AuthUser

    expect(
      hasPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.USER,
        ADMIN_PERMISSION_ACTIONS.READ
      )
    ).toBe(true)
  })

  test('ordinary-user editing does not inherit the admin baseline', () => {
    expect(normalizeAdminPermissions(undefined, catalog, null)).toEqual({
      [ADMIN_PERMISSION_RESOURCES.USER]: {
        [ADMIN_PERMISSION_ACTIONS.READ]: false,
      },
    })
  })
})
