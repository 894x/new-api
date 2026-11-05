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
import { useReducedMotion } from 'motion/react'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { MonitorRoute } from './api'
import { ChannelStatus } from './channel-status'
import { channelColor } from './route-data'

interface FlowGeometry {
  width: number
  height: number
  entry: string
  branches: string[]
}

export function RoutingFlow(props: {
  route: MonitorRoute
  at: number
  paused: boolean
  names?: Map<number, string>
  fill?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const reduceMotion = useReducedMotion()
  const id = useId().replaceAll(/[^a-zA-Z0-9]/g, '')
  const container = useRef<HTMLDivElement>(null)
  const [geometry, setGeometry] = useState<FlowGeometry>()
  const fingerprint = props.route.channels
    .map((channel) => channel.channel_id)
    .join(',')
  useLayoutEffect(() => {
    const element = container.current
    if (!element) return
    function measure() {
      if (!element) return
      const bounds = element.getBoundingClientRect()
      const source = element
        .querySelector('[data-flow-source]')
        ?.getBoundingClientRect()
      const policy = element
        .querySelector('[data-flow-policy]')
        ?.getBoundingClientRect()
      if (!source || !policy || bounds.width === 0) return
      const curve = (x1: number, y1: number, x2: number, y2: number) =>
        `M ${x1} ${y1} C ${(x1 + x2) / 2} ${y1}, ${(x1 + x2) / 2} ${y2}, ${x2} ${y2}`
      const entry = curve(
        source.right - bounds.left,
        source.y + source.height / 2 - bounds.top,
        policy.x - bounds.left,
        policy.y + policy.height / 2 - bounds.top
      )
      const branches = [...element.querySelectorAll('[data-flow-channel]')].map(
        (channel) => {
          const rect = channel.getBoundingClientRect()
          return curve(
            policy.right - bounds.left,
            policy.y + policy.height / 2 - bounds.top,
            rect.x - bounds.left,
            rect.y + rect.height / 2 - bounds.top
          )
        }
      )
      setGeometry({
        width: bounds.width,
        height: bounds.height,
        entry,
        branches,
      })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [fingerprint])
  const animate =
    !props.paused && !reduceMotion && props.route.selection_count > 0
  const mainShare = Math.max(
    ...props.route.channels.map((channel) => channel.target_share)
  )
  return (
    <div
      ref={container}
      className={cn(
        'relative grid min-w-0 grid-cols-1 items-center gap-7 px-4 py-6 md:grid-cols-[110px_165px_minmax(210px,260px)] md:justify-between md:gap-10',
        props.fill && 'xl:min-h-full xl:py-3'
      )}
    >
      {geometry && (
        <svg
          className='pointer-events-none absolute inset-0 hidden h-full w-full md:block'
          viewBox={`0 0 ${geometry.width} ${geometry.height}`}
          role='img'
          aria-label={t('Routing flow: line width represents target share')}
        >
          <path
            id={`${id}-entry`}
            d={geometry.entry}
            fill='none'
            stroke='var(--muted-foreground)'
            strokeWidth={2}
            opacity={0.25}
          />
          {animate && (
            <circle r={3} fill='var(--primary)'>
              <animateMotion dur='2s' repeatCount='indefinite'>
                <mpath href={`#${id}-entry`} />
              </animateMotion>
            </circle>
          )}
          {geometry.branches.map((path, index) => {
            const channel = props.route.channels[index]
            if (!channel) return null
            return (
              <g key={channel.channel_id}>
                <path
                  id={`${id}-channel-${channel.channel_id}`}
                  d={path}
                  fill='none'
                  stroke={channelColor(channel.channel_id)}
                  strokeWidth={Math.max(
                    1.5,
                    Math.sqrt(channel.target_share) * 8
                  )}
                  strokeDasharray={channel.probe ? '4 6' : undefined}
                  opacity={channel.target_share > 0 ? 0.8 : 0.2}
                />
                {animate &&
                  channel.target_share > 0 &&
                  Array.from(
                    {
                      length: Math.max(1, Math.ceil(channel.target_share * 4)),
                    },
                    (_, packet) => (
                      <circle
                        key={packet}
                        r={3}
                        fill={channelColor(channel.channel_id)}
                      >
                        <animateMotion
                          dur='3s'
                          begin={`-${packet * 0.75}s`}
                          repeatCount='indefinite'
                        >
                          <mpath
                            href={`#${id}-channel-${channel.channel_id}`}
                          />
                        </animateMotion>
                      </circle>
                    )
                  )}
              </g>
            )
          })}
        </svg>
      )}
      <Card
        data-flow-source
        className='relative mx-auto w-full max-w-48 text-center md:mx-0 md:max-w-none'
      >
        <CardContent>
          <p className='text-muted-foreground text-xs'>
            {t('Routing selections')}
          </p>
          <p className='mt-2 text-2xl font-medium tabular-nums'>
            {formatNumber(props.route.selection_count, locale)}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('Last {{seconds}} seconds', {
              seconds: props.route.window_seconds,
            })}
          </p>
        </CardContent>
      </Card>
      <Card
        data-flow-policy
        className='ring-primary/35 relative mx-auto w-full max-w-60 shadow-[0_0_0_5px_var(--muted)] md:mx-0 md:max-w-none'
      >
        <CardContent className='space-y-3'>
          <h3 className='font-medium'>
            {props.route.mode === 'discount'
              ? t('Discount-first routing')
              : t('Smart routing')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {props.route.mode === 'discount'
              ? t('Prefer lower discounts while meeting quality limits')
              : t('Adjust allocation using observed channel quality')}
          </p>
          <p className='border-t pt-2 text-xs'>
            {props.route.dynamic
              ? t('Adaptive allocation active')
              : t('Warming up')}
          </p>
        </CardContent>
      </Card>
      <div
        className='relative grid min-w-0 gap-3'
        role='list'
        aria-label={t('Candidate channels')}
      >
        {props.route.channels.map((channel) => (
          <Card
            key={channel.channel_id}
            data-flow-channel
            size='sm'
            role='listitem'
            className={cn(
              'gap-2',
              channel.target_share === mainShare && 'ring-primary/40'
            )}
          >
            <CardContent className='space-y-2'>
              <div className='flex flex-wrap items-center justify-between gap-1'>
                <span className='min-w-0 font-medium break-all'>
                  {props.names?.get(channel.channel_id) ||
                    t('Channel #{{id}}', { id: channel.channel_id })}
                </span>
                <ChannelStatus channel={channel} at={props.at} />
              </div>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <span className='text-muted-foreground text-xs'>
                  {t('Effective discount')}:{' '}
                  {channel.effective_cost_discount == null
                    ? '—'
                    : `×${formatNumber(channel.effective_cost_discount, locale)}`}
                </span>
                <span className='text-lg font-medium tabular-nums'>
                  <span className='text-muted-foreground mr-1 text-xs font-normal'>
                    {t('Target')}
                  </span>
                  {formatNumber(channel.target_share * 100, locale)}%
                </span>
              </div>
              <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-xs tabular-nums'>
                <span>
                  TTFT{' '}
                  {channel.ttft_ms == null
                    ? '—'
                    : `${formatNumber(channel.ttft_ms, locale)} ms`}
                </span>
                <span>
                  {t('Selected')}:{' '}
                  {formatNumber(channel.selected_share * 100, locale)}%
                </span>
              </div>
              <div className='bg-muted h-1 overflow-hidden rounded-full'>
                <div
                  className='h-full rounded-full'
                  style={{
                    width: `${Math.min(100, Math.max(0, channel.target_share * 100))}%`,
                    background: channelColor(channel.channel_id),
                  }}
                />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
