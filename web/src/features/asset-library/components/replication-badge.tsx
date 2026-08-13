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
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { AssetReplicaSummary } from '../types'

function getReplicationVariant(
  replication?: AssetReplicaSummary
): StatusVariant {
  if (!replication || replication.Total === 0) return 'neutral'
  if (replication.Failed > 0) return 'danger'
  if (replication.Processing > 0 || replication.Ready < replication.Total) {
    return 'warning'
  }
  return 'success'
}

export function ReplicationBadge(props: { replication?: AssetReplicaSummary }) {
  const { t } = useTranslation()
  const replication = props.replication
  const ready = replication?.Ready ?? 0
  const total = replication?.Total ?? 0
  let label = t('Not synchronized')
  if (total > 0) {
    label = t('{{ready}} of {{total}} channels ready', { ready, total })
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className='inline-flex max-w-full' tabIndex={0} role='status' />
        }
      >
        <StatusBadge
          label={label}
          variant={getReplicationVariant(replication)}
          copyable={false}
          className='-ml-1.5'
        />
      </TooltipTrigger>
      <TooltipContent>
        {t('Ready: {{ready}}, processing: {{processing}}, failed: {{failed}}', {
          ready,
          processing: replication?.Processing ?? 0,
          failed: replication?.Failed ?? 0,
        })}
      </TooltipContent>
    </Tooltip>
  )
}
