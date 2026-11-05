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
import {
  ArrowExpand02Icon,
  ArrowShrink02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export function RoutingWorkspaceDisplay(props: {
  toolbar: ReactNode
  refreshAction: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  const [fullscreen, setFullscreen] = useState(false)
  const [container] = useState(() => document.createElement('div'))
  const inlineContainer = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const target = fullscreen ? document.body : inlineContainer.current
    if (!target) return
    const focused = document.activeElement
    // Move a stable portal host so card drafts, focus and scroll survive toggles.
    container.className = 'contents'
    target.appendChild(container)
    if (focused instanceof HTMLElement && container.contains(focused)) {
      focused.focus({ preventScroll: true })
    }
    if (!fullscreen) return

    const background = [...document.body.children]
      .filter(
        (element): element is HTMLElement =>
          element instanceof HTMLElement && element !== container
      )
      .map((element) => ({ element, inert: element.inert }))
    const overflow = document.body.style.overflow
    background.forEach(({ element }) => {
      element.inert = true
    })
    document.body.style.overflow = 'hidden'
    return () => {
      background.forEach(({ element, inert }) => {
        element.inert = inert
      })
      document.body.style.overflow = overflow
    }
  }, [container, fullscreen])

  useEffect(() => () => container.remove(), [container])

  useEffect(() => {
    if (!fullscreen) return
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      // Let menus and editing dialogs consume Escape before leaving fullscreen.
      if (
        [
          ...document.querySelectorAll(
            '[data-slot="dialog-content"][data-open], [role="listbox"], [role="menu"]'
          ),
        ].some(
          (overlay) =>
            !overlay.closest(
              '[inert], [hidden], [aria-hidden="true"], [data-closed]'
            )
        )
      ) {
        return
      }
      setFullscreen(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [fullscreen])

  return (
    <div ref={inlineContainer} className='flex min-h-0 min-w-0 flex-1 flex-col'>
      {createPortal(
        <section
          role={fullscreen ? 'dialog' : 'region'}
          aria-modal={fullscreen || undefined}
          aria-label={t('Model routing')}
          className={cn(
            'flex min-h-0 min-w-0 flex-1 flex-col gap-4',
            fullscreen &&
              'bg-background fixed inset-0 z-40 overflow-hidden p-3 sm:p-4'
          )}
        >
          <div className='flex shrink-0 flex-wrap items-center gap-2'>
            {props.toolbar}
            <div className='flex shrink-0 items-center gap-2'>
              {props.refreshAction}
              <Button
                type='button'
                variant='outline'
                aria-pressed={fullscreen}
                onClick={() => setFullscreen((previous) => !previous)}
              >
                <HugeiconsIcon
                  icon={fullscreen ? ArrowShrink02Icon : ArrowExpand02Icon}
                  data-icon='inline-start'
                  aria-hidden='true'
                />
                {fullscreen ? t('Exit fullscreen') : t('Fullscreen')}
              </Button>
            </div>
          </div>
          {props.children}
        </section>,
        container
      )}
    </div>
  )
}
