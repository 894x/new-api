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
/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

import type { Asset, AssetGroup, AssetLibraryDialogType } from '../types'

type AssetLibraryContextValue = {
  open: AssetLibraryDialogType
  currentAsset: Asset | null
  currentGroup: AssetGroup | null
  targetUserId?: number
  isReadOnly: boolean
  openAssetDialog: (dialog: AssetLibraryDialogType, asset?: Asset) => void
  openGroupDialog: (dialog: AssetLibraryDialogType, group?: AssetGroup) => void
  closeDialog: () => void
}

const AssetLibraryContext = createContext<AssetLibraryContextValue | null>(null)

export function AssetLibraryProvider(props: {
  children: ReactNode
  targetUserId?: number
}) {
  const [open, setOpen] = useState<AssetLibraryDialogType>(null)
  const [currentAsset, setCurrentAsset] = useState<Asset | null>(null)
  const [currentGroup, setCurrentGroup] = useState<AssetGroup | null>(null)

  const closeDialog = useCallback(() => {
    setOpen(null)
    setCurrentAsset(null)
    setCurrentGroup(null)
  }, [])

  const openAssetDialog = useCallback(
    (dialog: AssetLibraryDialogType, asset?: Asset) => {
      setCurrentAsset(asset ?? null)
      setCurrentGroup(null)
      setOpen(dialog)
    },
    []
  )

  const openGroupDialog = useCallback(
    (dialog: AssetLibraryDialogType, group?: AssetGroup) => {
      setCurrentGroup(group ?? null)
      setCurrentAsset(null)
      setOpen(dialog)
    },
    []
  )

  const value = useMemo<AssetLibraryContextValue>(
    () => ({
      open,
      currentAsset,
      currentGroup,
      targetUserId: props.targetUserId,
      isReadOnly: props.targetUserId !== undefined,
      openAssetDialog,
      openGroupDialog,
      closeDialog,
    }),
    [
      open,
      currentAsset,
      currentGroup,
      props.targetUserId,
      openAssetDialog,
      openGroupDialog,
      closeDialog,
    ]
  )

  return (
    <AssetLibraryContext.Provider value={value}>
      {props.children}
    </AssetLibraryContext.Provider>
  )
}

export function useAssetLibrary() {
  const value = useContext(AssetLibraryContext)
  if (!value) {
    throw new Error('useAssetLibrary must be used within AssetLibraryProvider')
  }
  return value
}
