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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type TemplateActionsProps = {
  isAuthenticated: boolean
  inverse?: boolean
}

export function TemplateActions({
  isAuthenticated,
  inverse = false,
}: TemplateActionsProps) {
  const { t } = useTranslation()
  const primaryTarget = isAuthenticated ? '/dashboard' : '/sign-up'

  return (
    <div className='flex flex-wrap items-center gap-3'>
      <Button
        size='lg'
        className={inverse ? 'bg-white text-black hover:bg-white/90' : 'group'}
        render={<Link to={primaryTarget} />}
      >
        {isAuthenticated ? t('Go to Dashboard') : t('Get Started')}
        <ArrowRight className='ml-2 size-4' />
      </Button>
      <Button
        size='lg'
        variant='outline'
        className={
          inverse
            ? 'border-white/25 bg-transparent text-white hover:bg-white/10 hover:text-white'
            : undefined
        }
        render={<Link to='/pricing' />}
      >
        {t('View Pricing')}
      </Button>
    </div>
  )
}
