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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AdminAssetLibraryUserScope } from './components/admin-user-scope'
import { AssetLibraryDialogs } from './components/asset-library-dialogs'
import { AssetLibraryPrimaryButtons } from './components/asset-library-primary-buttons'
import { AssetLibraryProvider } from './components/asset-library-provider'
import { AssetsTable } from './components/assets-table'
import { GroupsTable } from './components/groups-table'

const route = getRouteApi('/_authenticated/asset-library/')

export function AssetLibrary() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const activeTab = search.tab || 'assets'
  const isAdmin = (user?.role ?? 0) >= ROLE.ADMIN
  const targetUserId =
    isAdmin && search.userId && search.userId !== user?.id
      ? search.userId
      : undefined

  const changeTargetUser = (userId?: number) => {
    navigate({
      search: (previous) => ({
        ...previous,
        userId,
        page: undefined,
        filter: undefined,
        assetType: undefined,
        groupId: undefined,
      }),
    })
  }

  return (
    <AssetLibraryProvider targetUserId={targetUserId}>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Asset Library')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {!targetUserId ? <AssetLibraryPrimaryButtons /> : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-3'>
            {isAdmin && user ? (
              <AdminAssetLibraryUserScope
                currentUser={user}
                targetUserId={targetUserId}
                onTargetUserIdChange={changeTargetUser}
              />
            ) : null}
            {targetUserId ? (
              <p className='text-muted-foreground text-sm'>
                {t('Viewing user #{{id}} in read-only mode.', {
                  id: targetUserId,
                })}
              </p>
            ) : null}
            <Tabs
              value={activeTab}
              onValueChange={(value) =>
                navigate({
                  search: (previous) => ({
                    ...previous,
                    tab: value as 'assets' | 'groups',
                    page: undefined,
                    filter: undefined,
                    assetType: undefined,
                    groupId: undefined,
                  }),
                })
              }
            >
              <TabsList>
                <TabsTrigger value='assets'>{t('Assets')}</TabsTrigger>
                <TabsTrigger value='groups'>{t('Asset Groups')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className='min-h-0 flex-1'>
              {activeTab === 'groups' ? <GroupsTable /> : <AssetsTable />}
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <AssetLibraryDialogs />
    </AssetLibraryProvider>
  )
}
