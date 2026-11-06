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
  useEffectEvent,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const fullscreenHosts: HTMLElement[] = []
const backgroundLeases = new Map<
  HTMLElement,
  { count: number; inert: boolean }
>()
let fullscreenBodyOverflow = ''

export function RoutingWorkspaceDisplay(props: {
  toolbar: ReactNode
  refreshAction: ReactNode
  children: ReactNode
  fullscreen?: boolean
  onFullscreenChange?: (fullscreen: boolean) => void
  hideToolbar?: boolean
  label?: string
  exitFullscreenLabel?: string
  toolbarActions?: ReactNode
}) {
  const { t } = useTranslation()
  const [localFullscreen, setLocalFullscreen] = useState(false)
  const fullscreen = props.fullscreen ?? localFullscreen
  const minimalInline = props.hideToolbar && !fullscreen
  const [container] = useState(() => document.createElement('div'))
  const inlineContainer = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const returnScroll = useRef<
    { element: HTMLElement; top: number; left: number }[]
  >([])
  const wasFullscreen = useRef(false)
  const changeFullscreen = (next: boolean) => {
    if (props.fullscreen === undefined) setLocalFullscreen(next)
    props.onFullscreenChange?.(next)
  }
  const notifyFullscreen = useEffectEvent(changeFullscreen)

  useLayoutEffect(() => {
    const target = fullscreen ? document.body : inlineContainer.current
    if (!target) return
    const focused = document.activeElement
    const returning = wasFullscreen.current && !fullscreen
    if (fullscreen && !wasFullscreen.current) {
      if (focused instanceof HTMLElement) returnFocus.current = focused
      returnScroll.current = []
      let ancestor = inlineContainer.current?.parentElement
      while (ancestor) {
        returnScroll.current.push({
          element: ancestor,
          top: ancestor.scrollTop,
          left: ancestor.scrollLeft,
        })
        ancestor = ancestor.parentElement
      }
    }
    wasFullscreen.current = fullscreen
    // Move a stable portal host so card drafts, focus and scroll survive toggles.
    container.className = 'contents'
    target.appendChild(container)
    if (returning && returnFocus.current?.isConnected) {
      returnFocus.current.focus({ preventScroll: true })
    } else if (focused instanceof HTMLElement && container.contains(focused)) {
      focused.focus({ preventScroll: true })
    }
    if (returning) {
      returnScroll.current.forEach(({ element, top, left }) => {
        if (!element.isConnected) return
        element.scrollTop = top
        element.scrollLeft = left
      })
      returnScroll.current = []
    }
    if (!fullscreen) return

    const background = [...document.body.children].filter(
      (element): element is HTMLElement =>
        element instanceof HTMLElement && element !== container
    )
    if (fullscreenHosts.length === 0) {
      fullscreenBodyOverflow = document.body.style.overflow
    }
    fullscreenHosts.push(container)
    background.forEach((element) => {
      const lease = backgroundLeases.get(element)
      backgroundLeases.set(element, {
        count: (lease?.count ?? 0) + 1,
        inert: lease?.inert ?? element.inert,
      })
      element.inert = true
    })
    document.body.style.overflow = 'hidden'
    return () => {
      background.forEach((element) => {
        const lease = backgroundLeases.get(element)
        if (!lease) return
        if (lease.count > 1) {
          lease.count--
        } else {
          element.inert = lease.inert
          backgroundLeases.delete(element)
        }
      })
      const index = fullscreenHosts.indexOf(container)
      if (index >= 0) fullscreenHosts.splice(index, 1)
      if (fullscreenHosts.length === 0) {
        document.body.style.overflow = fullscreenBodyOverflow
      }
    }
  }, [container, fullscreen])

  useEffect(() => () => container.remove(), [container])

  useEffect(() => {
    if (!fullscreen) return
    const overlayEscapes = new WeakSet<KeyboardEvent>()
    function hasOpenOverlay() {
      return [
        ...document.querySelectorAll(
          '[data-slot="dialog-content"][data-open], [data-slot="popover-content"][data-open], [role="listbox"], [role="menu"]'
        ),
      ].some(
        (overlay) =>
          !overlay.closest(
            '[inert], [hidden], [aria-hidden="true"], [data-closed]'
          )
      )
    }
    function onKeyDownCapture(event: KeyboardEvent) {
      if (event.key === 'Escape' && hasOpenOverlay()) {
        // Base UI may remove its popup before the bubbling handler runs.
        overlayEscapes.add(event)
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (
        event.key !== 'Escape' ||
        event.defaultPrevented ||
        overlayEscapes.has(event)
      ) {
        return
      }
      if (fullscreenHosts.at(-1) !== container) return
      // Let menus and editing dialogs consume Escape before leaving fullscreen.
      if (hasOpenOverlay()) {
        return
      }
      event.preventDefault()
      notifyFullscreen(false)
    }
    window.addEventListener('keydown', onKeyDownCapture, true)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDownCapture, true)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [container, fullscreen])

  let regionRole: 'dialog' | 'region' | undefined = 'region'
  if (fullscreen) regionRole = 'dialog'
  else if (minimalInline) regionRole = undefined

  return (
    <div
      ref={inlineContainer}
      className={cn(
        'flex min-h-0 min-w-0 flex-1 flex-col',
        minimalInline && 'contents'
      )}
    >
      {createPortal(
        <section
          role={regionRole}
          aria-modal={fullscreen || undefined}
          aria-label={
            minimalInline ? undefined : (props.label ?? t('Model routing'))
          }
          className={cn(
            'flex min-h-0 min-w-0 flex-1 flex-col gap-4',
            minimalInline && 'contents',
            fullscreen &&
              'bg-background fixed inset-0 z-40 overflow-hidden p-3 sm:p-4'
          )}
        >
          {!props.hideToolbar && (
            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              {props.toolbar}
              <div className='flex shrink-0 items-center gap-2'>
                {props.toolbarActions}
                {props.refreshAction}
                <Button
                  type='button'
                  variant='outline'
                  aria-pressed={fullscreen}
                  onClick={() => changeFullscreen(!fullscreen)}
                >
                  <HugeiconsIcon
                    icon={fullscreen ? ArrowShrink02Icon : ArrowExpand02Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {fullscreen
                    ? (props.exitFullscreenLabel ?? t('Exit fullscreen'))
                    : t('Fullscreen')}
                </Button>
              </div>
            </div>
          )}
          {props.children}
        </section>,
        container
      )}
    </div>
  )
}
