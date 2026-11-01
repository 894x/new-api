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
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { getUserRoutingPool, userRoutingPoolKeys } from './api'

export function UserRoutingSummary(props: {
  userId: number
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const catalog = useQuery({
    queryKey: userRoutingPoolKeys.catalog(props.userId),
    queryFn: () => getUserRoutingPool(props.userId),
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const data = catalog.data
  const hasOverrides = !!data?.configured_models?.length
  return (
    <div className='space-y-3'>
      <p role='status' className='text-muted-foreground text-sm'>
        {data &&
          (hasOverrides
            ? t(
                'This user has dedicated routing overrides. Manage them in the routing pool workspace.'
              )
            : t('This user follows the default routing pools.'))}
      </p>
      {catalog.isError && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void catalog.refetch()}
        >
          {t('Retry')}
        </Button>
      )}
      <Button
        type='button'
        variant='outline'
        disabled={props.disabled}
        onClick={() =>
          void navigate({
            to: '/model-channel-matrix',
            search: { user_id: props.userId },
          })
        }
      >
        {t('Manage routing pools')}
      </Button>
    </div>
  )
}
