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
import { api, type ApiRequestConfig } from '@/lib/api'

import type {
  Asset,
  AssetGroupReplicaDetailsResult,
  AssetGroup,
  AssetGroupsPage,
  AssetMutationResult,
  AssetLibraryResponse,
  AssetLibrarySyncReport,
  AssetReplicaDetailsResult,
  AssetsPage,
  ChannelAssetLibraryConfig,
  ChannelAssetLibraryConfigInput,
  ListAssetLibraryRequest,
} from './types'

const ASSET_LIBRARY_VERSION = '2024-01-01'
const assetLibraryRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} satisfies ApiRequestConfig

function getAssetLibraryError<TResult>(
  response: AssetLibraryResponse<TResult>
): string | null {
  const error = response.ResponseMetadata?.Error
  if (!error) return null
  if (error.Code && error.Message) return `${error.Code}: ${error.Message}`
  return error.Message || error.Code || 'Asset library request failed'
}

async function callAssetLibrary<TResult>(
  action: string,
  payload: object
): Promise<TResult> {
  const response = await api.post<AssetLibraryResponse<TResult>>(
    '/api/asset-library',
    payload,
    {
      ...assetLibraryRequestConfig,
      params: { Action: action, Version: ASSET_LIBRARY_VERSION },
    }
  )
  const error = getAssetLibraryError(response.data)
  if (error) throw new Error(error)
  return response.data.Result
}

async function callAssetLibraryVoid(
  action: string,
  payload: object
): Promise<void> {
  const response = await api.post<AssetLibraryResponse<unknown>>(
    '/api/asset-library',
    payload,
    {
      ...assetLibraryRequestConfig,
      params: { Action: action, Version: ASSET_LIBRARY_VERSION },
    }
  )
  const error = getAssetLibraryError(response.data)
  if (error) throw new Error(error)
}

export function listAssetGroups(
  request: ListAssetLibraryRequest
): Promise<AssetGroupsPage> {
  return callAssetLibrary('ListAssetGroups', request)
}

export async function listAllAssetGroups(): Promise<AssetGroup[]> {
  const pageSize = 100
  const firstPage = await listAssetGroups({
    PageNumber: 1,
    PageSize: pageSize,
    SortBy: 'CreateTime',
    SortOrder: 'Asc',
  })
  const pageCount = Math.ceil(firstPage.TotalCount / pageSize)
  if (pageCount <= 1) return firstPage.Items

  const remainingPages = await Promise.all(
    Array.from({ length: pageCount - 1 }, (_, index) =>
      listAssetGroups({
        PageNumber: index + 2,
        PageSize: pageSize,
        SortBy: 'CreateTime',
        SortOrder: 'Asc',
      })
    )
  )

  return [firstPage, ...remainingPages].flatMap((page) => page.Items)
}

export function listAssets(
  request: ListAssetLibraryRequest
): Promise<AssetsPage> {
  return callAssetLibrary('ListAssets', request)
}

export async function listAllAssets(): Promise<Asset[]> {
  const pageSize = 100
  const firstPage = await listAssets({
    PageNumber: 1,
    PageSize: pageSize,
    SortBy: 'CreateTime',
    SortOrder: 'Asc',
  })
  const pageCount = Math.ceil(firstPage.TotalCount / pageSize)
  if (pageCount <= 1) return firstPage.Items

  const remainingPages: AssetsPage[] = []
  for (let pageNumber = 2; pageNumber <= pageCount; pageNumber += 1) {
    remainingPages.push(
      await listAssets({
        PageNumber: pageNumber,
        PageSize: pageSize,
        SortBy: 'CreateTime',
        SortOrder: 'Asc',
      })
    )
  }

  return [firstPage, ...remainingPages].flatMap((page) => page.Items)
}

export function getAssetGroup(id: string): Promise<AssetGroup> {
  return callAssetLibrary('GetAssetGroup', { Id: id })
}

export function getAsset(id: string): Promise<Asset> {
  return callAssetLibrary('GetAsset', { Id: id })
}

export function createAssetGroup(input: {
  Name: string
  Description?: string
  GroupType?: string
}): Promise<AssetMutationResult> {
  return callAssetLibrary<AssetMutationResult>('CreateAssetGroup', input)
}

export function createAsset(input: {
  GroupId: string
  URL: string
  AssetType: string
  Name?: string
}): Promise<AssetMutationResult> {
  return callAssetLibrary('CreateAsset', input)
}

export function updateAssetGroup(input: {
  Id: string
  Name?: string
  Description?: string
}): Promise<AssetMutationResult> {
  return callAssetLibrary('UpdateAssetGroup', input)
}

export function updateAsset(input: {
  Id: string
  Name?: string
}): Promise<AssetMutationResult> {
  return callAssetLibrary('UpdateAsset', input)
}

export function deleteAsset(id: string): Promise<void> {
  return callAssetLibraryVoid('DeleteAsset', { Id: id })
}

export function deleteAssetGroup(id: string): Promise<void> {
  return callAssetLibraryVoid('DeleteAssetGroup', { Id: id })
}

type AdminAssetLibraryEnvelope<TResult> = {
  success?: boolean
  message?: string
  data?: TResult
}

function unwrapAdminAssetLibrary<TResult>(
  value: AdminAssetLibraryEnvelope<TResult>
): TResult {
  if (value.success === false) {
    throw new Error(value.message || 'Asset library admin request failed')
  }
  if (value.data === undefined) {
    throw new Error('Asset library admin response is missing data')
  }
  return value.data
}

export async function getAdminAssetReplicaDetails(
  assetId: string
): Promise<AssetReplicaDetailsResult> {
  const response = await api.get<
    AdminAssetLibraryEnvelope<AssetReplicaDetailsResult>
  >(
    `/api/asset-library/admin/assets/${encodeURIComponent(assetId)}/replicas`,
    assetLibraryRequestConfig
  )
  return unwrapAdminAssetLibrary(response.data)
}

export async function syncAdminAssetReplicas(
  assetId: string,
  channelIds: number[] = []
): Promise<AssetLibrarySyncReport> {
  const response = await api.post<
    AdminAssetLibraryEnvelope<AssetLibrarySyncReport>
  >(
    `/api/asset-library/admin/assets/${encodeURIComponent(assetId)}/sync`,
    { channel_ids: channelIds },
    assetLibraryRequestConfig
  )
  return unwrapAdminAssetLibrary(response.data)
}

export async function getAdminAssetGroupReplicaDetails(
  groupId: string
): Promise<AssetGroupReplicaDetailsResult> {
  const response = await api.get<
    AdminAssetLibraryEnvelope<AssetGroupReplicaDetailsResult>
  >(
    `/api/asset-library/admin/groups/${encodeURIComponent(groupId)}/replicas`,
    assetLibraryRequestConfig
  )
  return unwrapAdminAssetLibrary(response.data)
}

export async function syncAdminAssetGroupReplicas(
  groupId: string,
  channelIds: number[] = []
): Promise<AssetLibrarySyncReport> {
  const response = await api.post<
    AdminAssetLibraryEnvelope<AssetLibrarySyncReport>
  >(
    `/api/asset-library/admin/groups/${encodeURIComponent(groupId)}/sync`,
    { channel_ids: channelIds },
    assetLibraryRequestConfig
  )
  return unwrapAdminAssetLibrary(response.data)
}

type ConfigEnvelope = {
  success?: boolean
  message?: string
  data?: ChannelAssetLibraryConfig
  Result?: ChannelAssetLibraryConfig
}

function getConfigBusinessError(value: ConfigEnvelope): string | null {
  return value.success === false
    ? value.message || 'Asset library configuration request failed'
    : null
}

function unwrapChannelConfig(
  value: ConfigEnvelope | ChannelAssetLibraryConfig
): ChannelAssetLibraryConfig {
  const error = getConfigBusinessError(value as ConfigEnvelope)
  if (error) throw new Error(error)
  if ('Result' in value && value.Result) return value.Result
  if ('data' in value && value.data) return value.data
  return value as ChannelAssetLibraryConfig
}

export async function getChannelAssetLibraryConfig(
  channelId: number
): Promise<ChannelAssetLibraryConfig | null> {
  try {
    const response = await api.get<ConfigEnvelope | ChannelAssetLibraryConfig>(
      `/api/channel/${channelId}/asset-library`,
      assetLibraryRequestConfig
    )
    return unwrapChannelConfig(response.data)
  } catch (error: unknown) {
    const status = (error as { response?: { status?: number } }).response
      ?.status
    if (status === 404) return null
    throw error
  }
}

export async function saveChannelAssetLibraryConfig(
  channelId: number,
  input: ChannelAssetLibraryConfigInput
): Promise<ChannelAssetLibraryConfig> {
  const response = await api.put<ConfigEnvelope | ChannelAssetLibraryConfig>(
    `/api/channel/${channelId}/asset-library`,
    input,
    assetLibraryRequestConfig
  )
  return unwrapChannelConfig(response.data)
}

export async function deleteChannelAssetLibraryConfig(
  channelId: number
): Promise<void> {
  const response = await api.delete<ConfigEnvelope>(
    `/api/channel/${channelId}/asset-library`,
    assetLibraryRequestConfig
  )
  const error = getConfigBusinessError(response.data)
  if (error) throw new Error(error)
}

export async function syncChannelAssetLibrary(
  channelId: number
): Promise<void> {
  const response = await api.post<ConfigEnvelope>(
    `/api/channel/${channelId}/asset-library/sync`,
    {},
    assetLibraryRequestConfig
  )
  const error = getConfigBusinessError(response.data)
  if (error) throw new Error(error)
}
