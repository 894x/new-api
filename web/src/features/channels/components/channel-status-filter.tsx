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

import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'

type ChannelStatusFilterProps = {
  showAll: boolean
  onShowAllChange: (showAll: boolean) => void
}

export function ChannelStatusFilter(props: ChannelStatusFilterProps) {
  const { t } = useTranslation()

  return (
    <ButtonGroup aria-label={t('Channels')}>
      <Button
        type='button'
        size='sm'
        variant={props.showAll ? 'outline' : 'default'}
        aria-pressed={!props.showAll}
        onClick={() => props.onShowAllChange(false)}
      >
        {t('Enabled')}
      </Button>
      <Button
        type='button'
        size='sm'
        variant={props.showAll ? 'default' : 'outline'}
        aria-pressed={props.showAll}
        onClick={() => props.onShowAllChange(true)}
      >
        {t('All')}
      </Button>
    </ButtonGroup>
  )
}
