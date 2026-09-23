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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { bootstrapAuthentication, resolveAuthentication } from '../auth-session'
import { SESSION_HINT_COOKIE_NAME } from '../session-hint'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('axios', async (importOriginal) => {
  const actual = await importOriginal<typeof import('axios')>()
  return { ...actual, default: { ...actual.default, create: () => ({ post }) } }
})

const bundle: AuthBundle = {
  access_token: 'fixture-access',
  token_type: 'Bearer',
  access_expires_at: 4_000_000_000,
  user: { id: 42, username: 'returning-user', role: 1 },
  session: {
    sid: 'fixture-session',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'fixture',
    created_at: 100,
    last_active_at: 100,
    expires_at: 4_000_000_000,
  },
}

beforeEach(() => {
  post.mockReset()
  useAuthStore.getState().auth.reset('idle')
  document.cookie = `${SESSION_HINT_COOKIE_NAME}=; Max-Age=0; Path=/`
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  document.cookie = `${SESSION_HINT_COOKIE_NAME}=; Max-Age=0; Path=/`
})

describe('session hints remain an optimization, not an authentication verdict', () => {
  test('hintless public boot skips refresh but protected resolution recovers a valid session', async () => {
    expect(await bootstrapAuthentication()).toEqual({ kind: 'anonymous' })
    expect(post).not.toHaveBeenCalled()
    expect(useAuthStore.getState().auth.bootstrapState).toBe('idle')

    post.mockResolvedValue({
      status: 200,
      data: { success: true, data: bundle },
    })
    expect(await resolveAuthentication()).toEqual({
      kind: 'authenticated',
      bundle,
    })
    expect(post).toHaveBeenCalledWith('/api/user/auth/refresh', undefined, {
      headers: undefined,
    })
    expect(useAuthStore.getState().auth.accessToken).toBe(bundle.access_token)
  })

  test('a stale hint still requires a server verdict and cannot authenticate a visitor', async () => {
    document.cookie = `${SESSION_HINT_COOKIE_NAME}=1; Path=/`
    post.mockResolvedValue({ status: 401, data: {} })
    expect(await bootstrapAuthentication()).toEqual({ kind: 'anonymous' })
    expect(post).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().auth.user).toBeNull()
    expect(useAuthStore.getState().auth.bootstrapState).toBe('complete')
  })

  test('an existing valid in-memory session does not need a hint or network refresh', async () => {
    useAuthStore.getState().auth.setBundle(bundle)
    expect(await bootstrapAuthentication()).toEqual({
      kind: 'authenticated',
      bundle,
    })
    expect(post).not.toHaveBeenCalled()
  })

  test('an expired in-memory session is refreshed even without a hint', async () => {
    useAuthStore.getState().auth.setBundle({ ...bundle, access_expires_at: 1 })
    post.mockResolvedValue({
      status: 200,
      data: { success: true, data: bundle },
    })
    expect(await bootstrapAuthentication()).toEqual({
      kind: 'authenticated',
      bundle,
    })
    expect(post).toHaveBeenCalledWith('/api/user/auth/refresh', undefined, {
      headers: { 'X-Auth-Session': bundle.session.sid },
    })
  })
})
