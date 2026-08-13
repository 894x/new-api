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
import { AssetDeleteDialog, GroupDeleteDialog } from './asset-delete-dialog'
import { useAssetLibrary } from './asset-library-provider'
import { AssetMutateDialog } from './asset-mutate-dialog'
import { AssetPreviewDialog } from './asset-preview-dialog'
import { GroupMutateDialog } from './group-mutate-dialog'

export function AssetLibraryDialogs() {
  const { open, currentAsset, currentGroup, closeDialog } = useAssetLibrary()

  return (
    <>
      <AssetMutateDialog
        open={open === 'create-asset' || open === 'update-asset'}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        asset={open === 'update-asset' ? currentAsset : null}
      />
      <AssetPreviewDialog
        open={open === 'preview-asset'}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        asset={currentAsset}
      />
      <AssetDeleteDialog
        open={open === 'delete-asset'}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        asset={currentAsset}
      />
      <GroupMutateDialog
        open={open === 'create-group' || open === 'update-group'}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        group={open === 'update-group' ? currentGroup : null}
      />
      <GroupDeleteDialog
        open={open === 'delete-group'}
        onOpenChange={(isOpen) => !isOpen && closeDialog()}
        group={currentGroup}
      />
    </>
  )
}
