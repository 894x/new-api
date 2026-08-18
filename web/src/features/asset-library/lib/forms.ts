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
import { z } from 'zod'

import type {
  ChannelAssetLibraryAuthType,
  ChannelAssetLibraryBackend,
  ChannelAssetLibraryConfig,
  ChannelAssetLibraryConfigInput,
} from '../types'

export const assetFormSchema = z.object({
  name: z.string().max(64, 'Asset name must be 64 characters or fewer'),
  groupId: z.string().min(1, 'Select an asset group'),
  assetType: z.enum(['Image', 'Video', 'Audio']),
  url: z
    .string()
    .min(1, 'Public URL is required')
    .url('Enter a valid public URL')
    .refine((value) => /^https?:\/\//i.test(value), 'Use an HTTP or HTTPS URL'),
})

export const assetUpdateFormSchema = z.object({
  name: z.string().max(64, 'Asset name must be 64 characters or fewer'),
})

export const assetGroupFormSchema = z.object({
  name: z
    .string()
    .min(1, 'Group name is required')
    .max(64, 'Group name must be 64 characters or fewer'),
  description: z
    .string()
    .max(300, 'Description must be 300 characters or fewer'),
})

export const channelAssetConfigFormSchema = z
  .object({
    enabled: z.boolean(),
    backend: z.enum(['volcengine', 'seedance_sls', 'openapi']),
    baseUrl: z
      .string()
      .url('Enter a valid asset service URL')
      .or(z.literal('')),
    authType: z.enum(['aksk', 'bearer']),
    accessKey: z.string(),
    secretKey: z.string(),
    apiKey: z.string(),
    region: z.string(),
    projectName: z.string(),
    hasAccessKey: z.boolean(),
    hasSecretKey: z.boolean(),
    hasApiKey: z.boolean(),
  })
  .superRefine((value, context) => {
    if (!value.enabled) return
    if (!value.baseUrl.trim()) {
      context.addIssue({
        code: 'custom',
        path: ['baseUrl'],
        message: 'Asset service URL is required',
      })
    }
    if (value.backend !== 'volcengine' && value.authType !== 'bearer') {
      context.addIssue({
        code: 'custom',
        path: ['authType'],
        message: 'Selected asset backend requires Bearer authentication',
      })
    }
    if (value.authType === 'aksk') {
      if (!value.accessKey.trim() && !value.hasAccessKey) {
        context.addIssue({
          code: 'custom',
          path: ['accessKey'],
          message: 'Access Key is required',
        })
      }
      if (!value.secretKey.trim() && !value.hasSecretKey) {
        context.addIssue({
          code: 'custom',
          path: ['secretKey'],
          message: 'Secret Key is required',
        })
      }
    }
    if (
      value.authType === 'bearer' &&
      !value.apiKey.trim() &&
      !value.hasApiKey
    ) {
      context.addIssue({
        code: 'custom',
        path: ['apiKey'],
        message: 'API key is required',
      })
    }
  })

export type AssetFormValues = z.infer<typeof assetFormSchema>
export type AssetUpdateFormValues = z.infer<typeof assetUpdateFormSchema>
export type AssetGroupFormValues = z.infer<typeof assetGroupFormSchema>
export type ChannelAssetConfigFormValues = z.infer<
  typeof channelAssetConfigFormSchema
>

export function getChannelAssetBackendDefaults(
  backend: ChannelAssetLibraryBackend
): { baseUrl: string; authType: ChannelAssetLibraryAuthType } {
  switch (backend) {
    case 'seedance_sls':
      return { baseUrl: 'https://lm.sls.cn', authType: 'bearer' }
    case 'openapi':
      return { baseUrl: 'https://token.wxkjwlw.com', authType: 'bearer' }
    default:
      return {
        baseUrl: 'https://ark.cn-beijing.volcengineapi.com',
        authType: 'aksk',
      }
  }
}

export const DEFAULT_CHANNEL_ASSET_CONFIG_FORM_VALUES: ChannelAssetConfigFormValues =
  {
    enabled: false,
    backend: 'volcengine',
    baseUrl: 'https://ark.cn-beijing.volcengineapi.com',
    authType: 'aksk',
    accessKey: '',
    secretKey: '',
    apiKey: '',
    region: 'cn-beijing',
    projectName: 'default',
    hasAccessKey: false,
    hasSecretKey: false,
    hasApiKey: false,
  }

export function getChannelAssetConfigFormValues(
  config: ChannelAssetLibraryConfig | null
): ChannelAssetConfigFormValues {
  if (!config) return DEFAULT_CHANNEL_ASSET_CONFIG_FORM_VALUES

  return {
    enabled: config.enabled,
    backend: config.backend ?? 'volcengine',
    baseUrl: config.base_url,
    authType: config.auth_type,
    accessKey: '',
    secretKey: '',
    apiKey: '',
    region: config.region,
    projectName: config.project_name,
    hasAccessKey: config.has_access_key ?? false,
    hasSecretKey: config.has_secret_key ?? false,
    hasApiKey: config.has_api_key ?? false,
  }
}

export function getChannelAssetConfigPayload(
  values: ChannelAssetConfigFormValues
): ChannelAssetLibraryConfigInput {
  const payload: ChannelAssetLibraryConfigInput = {
    enabled: values.enabled,
    backend: values.backend,
    base_url: values.baseUrl.trim(),
    auth_type: values.authType,
    region: values.region.trim(),
    project_name: values.projectName.trim(),
  }

  if (values.accessKey.trim()) payload.access_key = values.accessKey.trim()
  if (values.secretKey.trim()) payload.secret_key = values.secretKey.trim()
  if (values.apiKey.trim()) payload.api_key = values.apiKey.trim()

  return payload
}

export function channelAssetConfigDestinationChanged(
  values: Pick<
    ChannelAssetConfigFormValues,
    'backend' | 'baseUrl' | 'authType'
  >,
  config: ChannelAssetLibraryConfig | null
) {
  if (!config) return false

  return (
    values.backend !== config.backend ||
    values.baseUrl.trim().replace(/\/+$/, '') !==
      config.base_url.trim().replace(/\/+$/, '') ||
    values.authType !== config.auth_type
  )
}
