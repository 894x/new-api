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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import type { DNSPodConfigState, DNSPodConfigUpdate } from '../types'

export function dnsTrafficSchema(t: TFunction) {
  const message = t(
    'Enter an integer weight from 1 to 100 for each enabled IP.'
  )
  return z.object({
    revision: z.string(),
    weights: z.array(
      z.object({
        id: z.string(),
        weight: z
          .number({ error: message })
          .int(message)
          .min(1, message)
          .max(100, message),
      })
    ),
  })
}

export type DNSTrafficForm = z.infer<ReturnType<typeof dnsTrafficSchema>>
export type DNSPodConfigForm = z.infer<ReturnType<typeof dnsPodConfigSchema>>

export function dnsPodConfigSchema(t: TFunction, state: DNSPodConfigState) {
  const invalid = t(
    'Enter a valid DNSPod domain, hostname, line and A or AAAA record type.'
  )
  const name =
    /^(?:[a-z\d](?:[a-z\d-]*[a-z\d])?)(?:\.[a-z\d](?:[a-z\d-]*[a-z\d])?)*$/i
  return z
    .object({
      revision: z.string(),
      enabled: z.boolean(),
      domain: z.string().max(253, invalid),
      subdomain: z.string().max(253, invalid),
      record_line: z.string().max(128, invalid),
      record_type: z.enum(['A', 'AAAA']),
      secret_id: z.string().max(256),
      secret_key: z.string().max(256),
      session_token: z.string().max(8192),
      clear_session_token: z.boolean(),
    })
    .superRefine((values, ctx) => {
      if (values.enabled || values.domain || values.subdomain) {
        if (
          !name.test(values.domain) ||
          !values.domain.includes('.') ||
          values.domain.split('.').some((part) => part.length > 63)
        ) {
          ctx.addIssue({ code: 'custom', path: ['domain'], message: invalid })
        }
        if (
          values.subdomain !== '@' &&
          (!name.test(values.subdomain) ||
            values.subdomain.split('.').some((part) => part.length > 63) ||
            values.domain.length + values.subdomain.length + 1 > 253)
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['subdomain'],
            message: invalid,
          })
        }
        if (!values.record_line.trim()) {
          ctx.addIssue({
            code: 'custom',
            path: ['record_line'],
            message: invalid,
          })
        }
      }
      for (const key of [
        'secret_id',
        'secret_key',
        'session_token',
        'record_line',
      ] as const) {
        if (
          values[key].trim() !== values[key] ||
          ['\r', '\n', '\0'].some((character) =>
            values[key].includes(character)
          )
        ) {
          ctx.addIssue({ code: 'custom', path: [key], message: invalid })
        }
      }
      if (values.enabled) {
        if (!values.secret_id && !state.secret_id_set) {
          ctx.addIssue({
            code: 'custom',
            path: ['secret_id'],
            message: t(
              'DNSPod credentials are required when enabling DNS management.'
            ),
          })
        }
        if (!values.secret_key && !state.secret_key_set) {
          ctx.addIssue({
            code: 'custom',
            path: ['secret_key'],
            message: t(
              'DNSPod credentials are required when enabling DNS management.'
            ),
          })
        }
      }
      if (values.clear_session_token && values.session_token) {
        ctx.addIssue({
          code: 'custom',
          path: ['session_token'],
          message: t('Enter a session token or clear it, not both.'),
        })
      }
    })
}

export async function dnsPodConfigChangeHash(update: DNSPodConfigUpdate) {
  const encoder = new TextEncoder()
  const values = [
    update.revision,
    String(update.enabled),
    update.domain,
    update.subdomain,
    update.record_line,
    update.record_type,
    update.secret_id,
    update.secret_key,
    update.session_token,
    String(update.clear_session_token),
  ]
  const canonical = values
    .map((value) => `${encoder.encode(value).length}:${value}`)
    .join('')
  const digest = await crypto.subtle.digest(
    'SHA-256',
    encoder.encode(canonical)
  )
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}
