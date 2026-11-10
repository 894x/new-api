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
  ArrowDown01Icon,
  ArrowUp01Icon,
  Cancel01Icon,
  Drag01Icon,
  Settings02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { motion, Reorder, useDragControls } from 'motion/react'
import {
  useEffect,
  useEffectEvent,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

type AutoGroupOrderItemProps = {
  group: string
  label?: ReactNode
  className?: string
  labelClassName?: string
  removeClassName?: string
  compact?: boolean
  reorderable?: boolean
  reorderLabel?: string
  disabled?: boolean
  index: number
  count: number
  leading?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  settings?: ReactNode
  settingsLabel?: string
  settingsOpenRequest?: number
  settingsTriggerId?: string
  onSettingsOpenChange?: (open: boolean, restoreFocus?: boolean) => void
  onDrop?: (point: { x: number; y: number }) => void
  onDraggingChange?: (dragging: boolean) => void
  onMove: (index: number, direction: 'up' | 'down') => void
  onRemove?: (group: string) => void
}

export function AutoGroupOrderItem(props: AutoGroupOrderItemProps) {
  const { t } = useTranslation()
  const dragControls = useDragControls()
  const reorderLabel = props.reorderLabel ?? props.group
  const [settingsOpen, setSettingsOpen] = useState(false)
  const notifySettingsOpen = useEffectEvent((open: boolean) =>
    props.onSettingsOpenChange?.(open)
  )
  useEffect(() => {
    if (props.settingsOpenRequest) {
      setSettingsOpen(true)
      notifySettingsOpen(true)
    }
  }, [props.settingsOpenRequest])

  const handleDragStart = (event: PointerEvent<HTMLButtonElement>) => {
    if (props.disabled) return
    dragControls.start(event)
  }

  const handleDragKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (props.disabled) return
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      props.onMove(props.index, 'up')
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      props.onMove(props.index, 'down')
    }
  }

  const itemClassName = cn(
    'bg-background relative flex min-w-0 items-center rounded-lg border',
    props.compact ? '@container/order-item gap-1 p-1' : 'gap-2 p-2',
    props.footer && 'flex-wrap',
    props.className
  )
  const content = (
    <>
      {(props.reorderable !== false || props.onDrop) && (
        <Button
          type='button'
          variant='ghost'
          size={props.compact ? 'icon-xs' : 'icon-sm'}
          className='text-muted-foreground cursor-grab touch-none font-mono active:cursor-grabbing'
          aria-label={t('Drag {{group}} to reorder', { group: reorderLabel })}
          title={t('Drag {{group}} to reorder', { group: reorderLabel })}
          onPointerDown={handleDragStart}
          onKeyDown={handleDragKeyDown}
          disabled={props.disabled}
        >
          <HugeiconsIcon icon={Drag01Icon} strokeWidth={2} aria-hidden='true' />
        </Button>
      )}

      {props.leading}
      <div
        className={cn(
          'flex min-w-0 flex-1 items-center',
          props.compact ? 'flex-nowrap gap-1.5' : 'flex-wrap gap-2'
        )}
      >
        <div
          className={cn(
            'min-w-0 truncate text-sm font-medium',
            props.labelClassName
          )}
          title={props.group}
        >
          {props.label ?? props.group}
        </div>
        {props.children}
      </div>
      <div
        className={cn('flex shrink-0 items-center', !props.compact && 'gap-1')}
      >
        {props.reorderable !== false && (
          <div
            className={cn('flex flex-row shrink-0', !props.compact && 'gap-1')}
          >
            <Button
              type='button'
              variant='ghost'
              size={props.compact ? 'icon-xs' : 'icon-sm'}
              disabled={props.disabled || props.index === 0}
              aria-label={t('Move {{group}} up', { group: reorderLabel })}
              onClick={() => props.onMove(props.index, 'up')}
            >
              <HugeiconsIcon
                icon={ArrowUp01Icon}
                strokeWidth={2}
                aria-hidden='true'
              />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size={props.compact ? 'icon-xs' : 'icon-sm'}
              disabled={props.disabled || props.index === props.count - 1}
              aria-label={t('Move {{group}} down', { group: reorderLabel })}
              onClick={() => props.onMove(props.index, 'down')}
            >
              <HugeiconsIcon
                icon={ArrowDown01Icon}
                strokeWidth={2}
                aria-hidden='true'
              />
            </Button>
          </div>
        )}
        {props.onRemove && (
          <Button
            type='button'
            variant='ghost'
            size={props.compact ? 'icon-xs' : 'icon-sm'}
            aria-label={t('Remove {{group}}', { group: props.group })}
            className={props.removeClassName}
            disabled={props.disabled}
            onClick={() => props.onRemove?.(props.group)}
          >
            <HugeiconsIcon
              icon={Cancel01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
          </Button>
        )}
        {props.settings && (
          <Popover
            open={settingsOpen}
            onOpenChange={(open, details) => {
              const restoreFocus =
                !open &&
                (details.reason === 'escape-key' ||
                  details.reason === 'trigger-press')
              setSettingsOpen(open)
              props.onSettingsOpenChange?.(open, restoreFocus)
            }}
          >
            <PopoverTrigger
              id={props.settingsTriggerId}
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size={props.compact ? 'icon-xs' : 'icon-sm'}
                  aria-label={props.settingsLabel ?? t('Settings')}
                  title={props.settingsLabel ?? t('Settings')}
                />
              }
            >
              <HugeiconsIcon
                icon={Settings02Icon}
                strokeWidth={2}
                aria-hidden='true'
              />
            </PopoverTrigger>
            <PopoverContent
              keepMounted
              align='end'
              sideOffset={8}
              role='dialog'
              className='max-h-[min(36rem,calc(100dvh-2rem))] w-[min(18rem,calc(100vw-1.5rem))] gap-3 overflow-y-auto overscroll-contain p-4'
            >
              <PopoverTitle className='break-all'>
                {props.settingsLabel ?? t('Settings')}
              </PopoverTitle>
              {props.settings}
            </PopoverContent>
          </Popover>
        )}
      </div>
      {props.footer && (
        <div className='w-full min-w-0 basis-full px-1 pt-1 pb-0.5'>
          {props.footer}
        </div>
      )}
    </>
  )
  if (props.reorderable === false) {
    if (props.onDrop) {
      return (
        <motion.li
          data-order-item={props.group}
          drag={props.disabled ? false : 'y'}
          dragControls={dragControls}
          dragListener={false}
          dragMomentum={false}
          dragSnapToOrigin
          whileDrag={{ zIndex: 20 }}
          className={itemClassName}
          onDragStart={() => props.onDraggingChange?.(true)}
          onDragEnd={(event, info) => {
            props.onDraggingChange?.(false)
            props.onDrop?.({
              x:
                'clientX' in event
                  ? event.clientX
                  : info.point.x - window.scrollX,
              y:
                'clientY' in event
                  ? event.clientY
                  : info.point.y - window.scrollY,
            })
          }}
        >
          {content}
        </motion.li>
      )
    }
    return <li className={itemClassName}>{content}</li>
  }
  return (
    <Reorder.Item
      value={props.group}
      dragListener={false}
      dragMomentum={false}
      dragControls={dragControls}
      className={itemClassName}
    >
      {content}
    </Reorder.Item>
  )
}
