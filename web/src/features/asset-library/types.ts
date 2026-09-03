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
export type AssetType = 'Image' | 'Video' | 'Audio'

export type AssetReplicaSummary = {
  Status: string
  Ready: number
  Processing: number
  Failed: number
  Total: number
}

export type AssetGroup = {
  Id: string
  Name: string
  Description?: string
  GroupType: string
  ProjectName: string
  CreateTime: string
  UpdateTime: string
  Replication?: AssetReplicaSummary
}

export type Asset = {
  Id: string
  Name?: string
  URL?: string
  GroupId: string
  AssetType: AssetType | string
  Format?: string
  FileSize?: number
  Width?: number
  Height?: number
  Duration?: number
  FPS?: number
  Status?: string
  Error?: {
    Code?: string
    Message?: string
  }
  ProjectName: string
  CreateTime: string
  UpdateTime: string
  LastInferenceTime?: string
  Replication?: AssetReplicaSummary
}

export type AssetLibraryResponseMetadata = {
  RequestId?: string
  Error?: {
    Code?: string
    Message?: string
  }
}

export type AssetLibraryResponse<TResult> = {
  ResponseMetadata?: AssetLibraryResponseMetadata
  Result: TResult
}

export type AssetMutationResult = {
  Id: string
  Replication?: AssetReplicaSummary
}

export type AssetGroupsPage = {
  TotalCount: number
  Items: AssetGroup[]
  PageNumber: number
  PageSize: number
}

export type AssetsPage = {
  TotalCount: number
  Items: Asset[]
  PageNumber: number
  PageSize: number
}

export type AssetLibraryFilter = {
  GroupIds?: string[]
  GroupType?: string
  Statuses?: string[]
  Name?: string
  AssetType?: AssetType
}

export type ListAssetLibraryRequest = {
  Filter?: AssetLibraryFilter
  PageNumber?: number
  PageSize?: number
  SortBy?: 'CreateTime' | 'UpdateTime' | 'GroupId'
  SortOrder?: 'Asc' | 'Desc'
  ProjectName?: string
}

export type ChannelAssetLibraryAuthType = 'aksk' | 'bearer'
export type ChannelAssetLibraryBackend =
  | 'volcengine'
  | 'seedance_sls'
  | 'openapi'

export type ChannelAssetLibraryConfig = {
  channel_id?: number
  enabled: boolean
  backend: ChannelAssetLibraryBackend
  base_url: string
  auth_type: ChannelAssetLibraryAuthType
  access_key?: string
  secret_key?: string
  api_key?: string
  has_access_key?: boolean
  has_secret_key?: boolean
  has_api_key?: boolean
  replica_count?: number
  region: string
  project_name: string
  created_time?: number
  updated_time?: number
}

export type ChannelAssetLibraryConfigInput = Pick<
  ChannelAssetLibraryConfig,
  'enabled' | 'backend' | 'base_url' | 'auth_type' | 'region' | 'project_name'
> &
  Partial<
    Pick<ChannelAssetLibraryConfig, 'access_key' | 'secret_key' | 'api_key'>
  >

export type AssetLibraryDialogType =
  | 'create-asset'
  | 'update-asset'
  | 'delete-asset'
  | 'preview-asset'
  | 'create-group'
  | 'group-details'
  | 'update-group'
  | 'delete-group'
  | null

export type AssetChannelReplicaDetails = {
  channel_id: number
  channel_name: string
  backend: string
  enabled: boolean
  state: string
  upstream_asset_id: string
  upstream_status: string
  last_error_code?: string
  last_error?: string
  last_inference_time?: string
  updated_time?: number
}

export type AssetGroupChannelReplicaDetails = {
  channel_id: number
  channel_name: string
  backend: string
  enabled: boolean
  state: string
  upstream_group_id: string
  last_error?: string
  updated_time?: number
}

export type AssetReplicaDetailsResult = {
  asset: Asset
  summary: AssetReplicaSummary
  replicas: AssetChannelReplicaDetails[]
}

export type AssetGroupReplicaDetailsResult = {
  summary: AssetReplicaSummary
  replicas: AssetGroupChannelReplicaDetails[]
}

export type AssetLibrarySyncReport = {
  Summary: AssetReplicaSummary
  Errors: Array<{ channel_id: number; asset_id?: string; message: string }>
}
