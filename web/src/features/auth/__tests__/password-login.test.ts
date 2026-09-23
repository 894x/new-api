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
import {
  constants,
  generateKeyPairSync,
  privateDecrypt,
  webcrypto,
} from 'node:crypto'

import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { login } from '../api'
import { clearPasswordEncryptionCache } from '../lib/password-encryption'

const keys = generateKeyPairSync('rsa', { modulusLength: 2048 })
const publicPEM = keys.publicKey
  .export({ type: 'spki', format: 'pem' })
  .toString()
const password = '密码 with spaces & symbols!'

describe('password login transport', () => {
  beforeEach(() => {
    clearPasswordEncryptionCache()
  })

  afterEach(() => {
    clearPasswordEncryptionCache()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  test('keeps ordinary password login when encryption is not enabled', async () => {
    const get = vi.spyOn(api, 'get')
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, message: '' },
    })

    await login({ username: 'tester', password })

    expect(get).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledWith(
      '/api/user/login?turnstile=',
      { username: 'tester', password },
      { skipAuthRefresh: true }
    )
  })

  test.each(['webcrypto', 'forge', 'unsupported-webcrypto'])(
    'sends only decryptable OAEP SHA-256 ciphertext with %s',
    async (mode) => {
      if (mode === 'webcrypto') vi.stubGlobal('crypto', webcrypto)
      else if (mode === 'forge') vi.stubGlobal('crypto', {})
      else {
        vi.stubGlobal('crypto', {
          subtle: {
            importKey: vi.fn().mockRejectedValue(new Error('unsupported')),
          },
        })
      }
      const get = vi.spyOn(api, 'get').mockResolvedValue({
        data: {
          success: true,
          data: { kid: 'key-one', public_key: publicPEM },
        },
      })
      const post = vi.spyOn(api, 'post').mockResolvedValue({
        data: { success: true, message: '' },
      })

      const result = await login({
        username: 'tester',
        password,
        turnstile: 'challenge',
        passwordEncryptionEnabled: true,
      })

      expect(result.success).toBe(true)
      expect(get).toHaveBeenCalledWith('/api/user/login/encryption-key')
      expect(post).toHaveBeenCalledWith(
        '/api/user/login?turnstile=challenge',
        {
          username: 'tester',
          encryption_key_id: 'key-one',
          password_encrypted: expect.any(String),
        },
        { skipAuthRefresh: true }
      )
      const body = post.mock.calls[0][1] as { password_encrypted: string }
      const decrypted = privateDecrypt(
        {
          key: keys.privateKey,
          padding: constants.RSA_PKCS1_OAEP_PADDING,
          oaepHash: 'sha256',
        },
        Buffer.from(body.password_encrypted, 'base64')
      )
      expect(decrypted.toString('utf8')).toBe(password)
    }
  )

  test.each(['unavailable', 'malformed-key', 'disabled'])(
    'does not submit plaintext when encryption configuration is %s',
    async (failure) => {
      vi.stubGlobal('crypto', webcrypto)
      const get = vi.spyOn(api, 'get')
      if (failure === 'unavailable') get.mockRejectedValue(new Error('offline'))
      else {
        const data =
          failure === 'disabled'
            ? { enabled: false }
            : { kid: 'key-one', public_key: 'invalid PEM' }
        get.mockResolvedValue({ data: { success: true, data } })
      }
      const post = vi.spyOn(api, 'post')

      await expect(
        login({
          username: 'tester',
          password,
          passwordEncryptionEnabled: true,
        })
      ).rejects.toThrow()

      expect(post).not.toHaveBeenCalled()
    }
  )

  test.each(['rejected', 'network-error'])(
    'refreshes the public key on the next attempt after %s',
    async (failure) => {
      vi.stubGlobal('crypto', webcrypto)
      const get = vi
        .spyOn(api, 'get')
        .mockResolvedValueOnce({
          data: {
            success: true,
            data: { kid: 'old-key', public_key: publicPEM },
          },
        })
        .mockResolvedValueOnce({
          data: {
            success: true,
            data: { kid: 'new-key', public_key: publicPEM },
          },
        })
      const post = vi.spyOn(api, 'post')
      if (failure === 'network-error') {
        post.mockRejectedValueOnce(new Error('offline'))
      } else {
        post.mockResolvedValueOnce({
          data: { success: false, message: 'retry' },
        })
      }
      post.mockResolvedValueOnce({ data: { success: true, message: '' } })
      const payload = {
        username: 'tester',
        password,
        passwordEncryptionEnabled: true,
      }

      if (failure === 'network-error') {
        await expect(login(payload)).rejects.toThrow('offline')
      } else {
        expect((await login(payload)).success).toBe(false)
      }
      expect((await login(payload)).success).toBe(true)

      expect(get).toHaveBeenCalledTimes(2)
      expect(post.mock.calls[1][1]).toEqual({
        username: 'tester',
        encryption_key_id: 'new-key',
        password_encrypted: expect.any(String),
      })
    }
  )
})
