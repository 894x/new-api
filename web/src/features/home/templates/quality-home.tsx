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
  BadgeCheck,
  Check,
  CircleCheck,
  FileText,
  Headphones,
  MessageCircle,
  Route,
  SearchCheck,
  ShieldCheck,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TemplateActions } from './template-actions'

type QualityHomeProps = {
  isAuthenticated: boolean
}

function QualitySignalPanel() {
  const { t } = useTranslation()

  return (
    <div className='border-border/70 bg-card relative overflow-hidden rounded-2xl border shadow-[0_24px_80px_-40px_rgba(37,99,235,0.55)]'>
      <div className='border-border/70 flex items-center justify-between border-b px-5 py-4'>
        <div>
          <p className='text-sm font-semibold'>{t('Service quality')}</p>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t('What customers can verify')}
          </p>
        </div>
        <span className='inline-flex items-center gap-2 text-xs font-medium text-emerald-600 dark:text-emerald-400'>
          <span className='size-2 rounded-full bg-emerald-500' />
          {t('Available')}
        </span>
      </div>
      <div className='space-y-3 p-5'>
        {[
          {
            icon: ShieldCheck,
            title: t('Official upstream quality'),
            detail: t('Clear sourcing and accountable delivery'),
          },
          {
            icon: Route,
            title: t('No diluted routes'),
            detail: t('Quality-first routing instead of opaque substitutions'),
          },
          {
            icon: Headphones,
            title: t('Responsive after-sales support'),
            detail: t('Real people follow up when an issue occurs'),
          },
        ].map((item) => (
          <div
            key={item.title}
            className='border-border/60 bg-muted/25 flex items-start gap-3 rounded-xl border p-4'
          >
            <div className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg'>
              <item.icon className='size-4' />
            </div>
            <div className='min-w-0'>
              <p className='text-sm font-semibold'>{item.title}</p>
              <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
                {item.detail}
              </p>
            </div>
            <CircleCheck className='ml-auto size-4 shrink-0 text-emerald-500' />
          </div>
        ))}
      </div>
      <div className='border-border/70 bg-muted/20 grid grid-cols-3 border-t'>
        <div className='px-4 py-3'>
          <p className='text-muted-foreground text-[11px]'>{t('Priority')}</p>
          <p className='mt-1 text-xs font-semibold'>{t('Quality first')}</p>
        </div>
        <div className='border-border/70 border-x px-4 py-3'>
          <p className='text-muted-foreground text-[11px]'>{t('Support')}</p>
          <p className='mt-1 text-xs font-semibold'>{t('Human follow-up')}</p>
        </div>
        <div className='px-4 py-3'>
          <p className='text-muted-foreground text-[11px]'>{t('Billing')}</p>
          <p className='mt-1 text-xs font-semibold'>{t('Transparent')}</p>
        </div>
      </div>
    </div>
  )
}

export default function QualityHome({ isAuthenticated }: QualityHomeProps) {
  const { t } = useTranslation()

  return (
    <main className='overflow-hidden'>
      <section className='relative px-6 pt-28 pb-20 md:pt-36 md:pb-28'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10 opacity-35 dark:opacity-20'
          style={{
            background:
              'radial-gradient(ellipse 55% 45% at 18% 25%, oklch(0.72 0.15 250 / 0.55), transparent 72%), radial-gradient(ellipse 45% 40% at 82% 20%, oklch(0.7 0.14 285 / 0.4), transparent 72%)',
          }}
        />
        <div className='mx-auto grid max-w-6xl items-center gap-14 lg:grid-cols-[1.02fr_0.98fr]'>
          <div>
            <h1 className='max-w-3xl text-[clamp(2.7rem,6vw,4.8rem)] leading-[1.02] font-bold tracking-[-0.045em]'>
              {t('Official-grade access.')}
              <br />
              <span className='bg-gradient-to-r from-blue-500 via-violet-500 to-fuchsia-500 bg-clip-text text-transparent'>
                {t('No diluted routes.')}
              </span>
              <br />
              {t('Support when it matters.')}
            </h1>
            <p className='text-muted-foreground mt-7 max-w-xl text-base leading-8 md:text-lg'>
              {t(
                'A high-quality API relay for teams that value official-grade model access, stable delivery, and accountable support.'
              )}
            </p>
            <div className='mt-8'>
              <TemplateActions isAuthenticated={isAuthenticated} />
            </div>
            <div className='text-muted-foreground mt-7 flex flex-wrap gap-x-6 gap-y-2 text-sm'>
              <span className='inline-flex items-center gap-2'>
                <Check className='size-4 text-emerald-500' />
                {t('Unified API access')}
              </span>
              <span className='inline-flex items-center gap-2'>
                <Check className='size-4 text-emerald-500' />
                {t('Clear customer pricing')}
              </span>
              <span className='inline-flex items-center gap-2'>
                <Check className='size-4 text-emerald-500' />
                {t('After-sales support')}
              </span>
            </div>
          </div>
          <QualitySignalPanel />
        </div>
      </section>

      <section className='border-border/70 border-y'>
        <div className='mx-auto grid max-w-6xl divide-y px-6 md:grid-cols-3 md:divide-x md:divide-y-0'>
          {[
            {
              icon: ShieldCheck,
              title: t('Quality before shortcuts'),
              text: t(
                'Routing policies prioritize dependable delivery and make service boundaries explicit.'
              ),
            },
            {
              icon: BadgeCheck,
              title: t('Claims customers can verify'),
              text: t(
                'Pricing, model availability, and service rules stay visible instead of relying on vague promises.'
              ),
            },
            {
              icon: Headphones,
              title: t('Support that closes the loop'),
              text: t(
                'Issues are followed through with clear ownership, updates, and a practical resolution path.'
              ),
            },
          ].map((item) => (
            <article key={item.title} className='px-0 py-10 md:px-8'>
              <item.icon className='text-primary size-6' />
              <h2 className='mt-5 text-lg font-semibold'>{item.title}</h2>
              <p className='text-muted-foreground mt-3 text-sm leading-7'>
                {item.text}
              </p>
            </article>
          ))}
        </div>
      </section>

      <section className='px-6 py-24 md:py-32'>
        <div className='mx-auto grid max-w-6xl gap-14 lg:grid-cols-[0.8fr_1.2fr]'>
          <div>
            <p className='text-primary text-sm font-semibold'>
              {t('From access to support')}
            </p>
            <h2 className='mt-4 text-3xl leading-tight font-bold tracking-tight md:text-5xl'>
              {t('Every request has a clear service path.')}
            </h2>
          </div>
          <ol className='border-border divide-border border-y'>
            {[
              [
                '01',
                t('Connect through one compatible API'),
                t(
                  'Keep the integration surface consistent while the platform manages upstream access.'
                ),
              ],
              [
                '02',
                t('Route with quality as the priority'),
                t(
                  'Availability checks and routing policies select a suitable service path.'
                ),
              ],
              [
                '03',
                t('Escalate with real support'),
                t(
                  'When an issue needs attention, the support trail remains clear and accountable.'
                ),
              ],
            ].map(([number, title, text]) => (
              <li
                key={number}
                className='grid gap-3 border-b py-7 last:border-b-0 sm:grid-cols-[3rem_1fr]'
              >
                <span className='text-primary font-mono text-sm'>{number}</span>
                <div>
                  <h3 className='font-semibold'>{title}</h3>
                  <p className='text-muted-foreground mt-2 text-sm leading-7'>
                    {text}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section className='border-border/70 border-t px-6 py-24 md:py-28'>
        <div className='mx-auto max-w-6xl'>
          <div className='max-w-2xl'>
            <h2 className='text-3xl font-bold tracking-tight md:text-4xl'>
              {t('Support does not end after a request is forwarded.')}
            </h2>
            <p className='text-muted-foreground mt-4 text-base leading-8 md:text-lg'>
              {t(
                'When an issue occurs, there is a clear owner, a response path, and follow-up through resolution.'
              )}
            </p>
          </div>

          <ol className='mt-14 grid gap-10 md:grid-cols-4 md:gap-4'>
            {[
              {
                icon: MessageCircle,
                title: t('Integration consultation'),
                text: t(
                  'Share your scenario and requirements to receive integration guidance and documentation.'
                ),
              },
              {
                icon: FileText,
                title: t('Issue intake'),
                text: t(
                  'Submit the issue or error details so the case can be confirmed and recorded.'
                ),
              },
              {
                icon: SearchCheck,
                title: t('Technical investigation'),
                text: t(
                  'Identify the cause and coordinate with the upstream provider when needed.'
                ),
              },
              {
                icon: CircleCheck,
                title: t('Resolution follow-up'),
                text: t(
                  'Share the outcome and next steps, keeping the issue loop accountable.'
                ),
              },
            ].map((step) => (
              <li key={step.title}>
                <step.icon className='text-primary size-10 stroke-[1.5]' />
                <h3 className='mt-6 text-lg font-semibold'>{step.title}</h3>
                <p className='text-muted-foreground mt-3 max-w-[15rem] text-sm leading-7'>
                  {step.text}
                </p>
              </li>
            ))}
          </ol>

          <p className='text-muted-foreground mt-14 text-sm'>
            {t(
              'Every connection and every piece of feedback helps us keep improving the service experience.'
            )}
          </p>
        </div>
      </section>

      <section className='border-border/70 border-y px-6 py-20'>
        <div className='mx-auto flex max-w-6xl flex-col items-start justify-between gap-8 md:flex-row md:items-center'>
          <div>
            <h2 className='text-3xl font-bold tracking-tight md:text-4xl'>
              {t('Choose quality you can stand behind.')}
            </h2>
            <p className='text-muted-foreground mt-3 max-w-2xl'>
              {t(
                'Start with a unified API, transparent customer pricing, and support that stays accountable.'
              )}
            </p>
          </div>
          <TemplateActions isAuthenticated={isAuthenticated} />
        </div>
      </section>
    </main>
  )
}
