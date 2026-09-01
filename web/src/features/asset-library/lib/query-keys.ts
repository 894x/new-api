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
export const assetLibraryQueryKeys = {
  all: ['asset-library'] as const,
  assets: () => [...assetLibraryQueryKeys.all, 'assets'] as const,
  assetLists: () => [...assetLibraryQueryKeys.assets(), 'list'] as const,
  assetList: (params: object) =>
    [...assetLibraryQueryKeys.assetLists(), params] as const,
  asset: (id: string) => [...assetLibraryQueryKeys.assets(), id] as const,
  assetReplicas: (id: string) =>
    [...assetLibraryQueryKeys.asset(id), 'replicas'] as const,
  statusRefreshes: () =>
    [...assetLibraryQueryKeys.all, 'status-refresh'] as const,
  statusRefreshLibrary: () =>
    [...assetLibraryQueryKeys.statusRefreshes(), 'library'] as const,
  statusRefreshAsset: (id: string) =>
    [...assetLibraryQueryKeys.statusRefreshes(), 'asset', id] as const,
  groups: () => [...assetLibraryQueryKeys.all, 'groups'] as const,
  groupList: (params: object) =>
    [...assetLibraryQueryKeys.groups(), 'list', params] as const,
  groupOptions: () => [...assetLibraryQueryKeys.groups(), 'options'] as const,
  group: (id: string) => [...assetLibraryQueryKeys.groups(), id] as const,
  groupReplicas: (id: string) =>
    [...assetLibraryQueryKeys.group(id), 'replicas'] as const,
  channelConfig: (channelId: number) =>
    [...assetLibraryQueryKeys.all, 'channel-config', channelId] as const,
}
