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
import {
  ArrowRight,
  Check,
  CircleDollarSign,
  GitBranch,
  RefreshCw,
  Route,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { TemplateActions } from './template-actions'

type EconomyHomeProps = {
  isAuthenticated: boolean
}

const CODE_SAMPLES = {
  cURL: `curl -X POST "$API_BASE/v1/chat/completions" \\
  -H "Authorization: Bearer $API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4.1-mini","messages":[{"role":"user","content":"Hello"}]}'`,
  Python: `from openai import OpenAI

client = OpenAI(api_key=API_KEY, base_url=API_BASE)
result = client.chat.completions.create(
    model="gpt-4.1-mini",
    messages=[{"role": "user", "content": "Hello"}],
)`,
  'Node.js': `import OpenAI from "openai";

const client = new OpenAI({ apiKey: API_KEY, baseURL: API_BASE });
const result = await client.chat.completions.create({
  model: "gpt-4.1-mini",
  messages: [{ role: "user", content: "Hello" }],
});`,
} as const

type CodeLanguage = keyof typeof CODE_SAMPLES

function RoutingPanel() {
  const { t } = useTranslation()

  return (
    <div className='border-border bg-card overflow-hidden rounded-2xl border shadow-[0_24px_80px_-40px_rgba(109,40,217,0.6)]'>
      <div className='border-border flex items-center justify-between border-b px-5 py-4'>
        <span className='text-sm font-bold'>
          {t('Unified request routing')}
        </span>
        <span className='inline-flex items-center gap-2 text-xs text-emerald-600 dark:text-emerald-400'>
          <span className='size-2 rounded-full bg-emerald-500' />
          {t('Available')}
        </span>
      </div>
      <div className='grid gap-2 p-5 sm:grid-cols-3'>
        {[
          ['A', t('Primary route'), t('Best value'), true],
          ['B', t('Available route'), t('Balanced'), false],
          ['C', t('Fallback route'), t('Ready'), false],
        ].map(([id, name, status, active]) => (
          <div
            key={String(id)}
            className={`grid gap-4 rounded-xl border p-4 ${
              active ? 'border-violet-500/70 bg-violet-500/10' : 'border-border'
            }`}
          >
            <span className='border-border flex size-7 items-center justify-center rounded-full border font-mono text-xs font-bold'>
              {id}
            </span>
            <div className='min-w-0'>
              <p className='text-sm font-semibold'>{name}</p>
              <p className='text-muted-foreground text-xs'>
                {t('Availability checked')}
              </p>
            </div>
            <span className='bg-muted w-fit px-2 py-1 text-[11px] font-medium'>
              {status}
            </span>
          </div>
        ))}
      </div>
      <div className='border-border bg-muted/30 grid grid-cols-3 border-t text-center text-xs'>
        <span className='border-border border-r px-3 py-3'>
          {t('Low price')}
        </span>
        <span className='border-border border-r px-3 py-3'>
          {t('Balanced')}
        </span>
        <span className='bg-violet-500 px-3 py-3 font-semibold text-white'>
          {t('Stable first')}
        </span>
      </div>
    </div>
  )
}

function CodeExample() {
  const { t } = useTranslation()
  const [language, setLanguage] = useState<CodeLanguage>('cURL')

  return (
    <div className='border-border overflow-hidden rounded-xl border bg-[#101114] text-zinc-100 shadow-[8px_8px_0_rgba(124,58,237,0.65)]'>
      <div className='flex items-center gap-1 border-b border-white/10 px-4'>
        {(Object.keys(CODE_SAMPLES) as CodeLanguage[]).map((item) => (
          <button
            key={item}
            type='button'
            onClick={() => setLanguage(item)}
            className={`border-b-2 px-3 py-3 text-xs transition-colors ${
              language === item
                ? 'border-violet-400 text-violet-400'
                : 'border-transparent text-zinc-400 hover:text-white'
            }`}
          >
            {item}
          </button>
        ))}
      </div>
      <pre className='min-h-64 overflow-x-auto p-6 text-xs leading-6'>
        <code>{CODE_SAMPLES[language]}</code>
      </pre>
      <div className='border-t border-white/10 px-6 py-3 text-xs text-zinc-500'>
        {t('Use the same API contract while routing stays platform-managed.')}
      </div>
    </div>
  )
}

export default function EconomyHome({ isAuthenticated }: EconomyHomeProps) {
  const { t } = useTranslation()

  return (
    <main className='overflow-hidden'>
      <section className='px-6 pt-28 pb-20 md:pt-36 md:pb-28'>
        <div className='mx-auto grid max-w-6xl items-center gap-14 lg:grid-cols-[0.92fr_1.08fr]'>
          <div>
            <h1 className='text-[clamp(3rem,6.5vw,5.5rem)] leading-[0.93] font-black tracking-[-0.065em]'>
              {t('Same model.')}
              <br />
              <span className='bg-gradient-to-r from-blue-500 via-violet-500 to-fuchsia-500 bg-clip-text text-transparent'>
                {t('More routes.')}
              </span>
              <br />
              {t('Better pricing.')}
            </h1>
            <p className='text-muted-foreground mt-7 max-w-xl text-base leading-8 md:text-lg'>
              {t(
                'Aggregate multiple available upstream routes behind one compatible API, then route by availability and priority to reduce access costs.'
              )}
            </p>
            <div className='mt-8'>
              <TemplateActions isAuthenticated={isAuthenticated} />
            </div>
            <p className='text-muted-foreground mt-5 text-xs'>
              {t(
                'Customer pricing is public. Internal channel costs stay private.'
              )}
            </p>
          </div>
          <RoutingPanel />
        </div>
      </section>

      <section className='border-border grid border-y md:grid-cols-4'>
        {[
          [Route, t('One API'), t('Compatible model access')],
          [GitBranch, t('Multiple routes'), t('Expandable upstream pool')],
          [RefreshCw, t('Unified routing'), t('Priority and retry policies')],
          [
            CircleDollarSign,
            t('Transparent billing'),
            t('Clear customer-facing pricing'),
          ],
        ].map(([Icon, title, text], index) => {
          const FeatureIcon = Icon as typeof Route
          return (
            <article
              key={String(title)}
              className={`flex items-center gap-4 px-6 py-7 ${
                index > 0
                  ? 'border-border border-t md:border-t-0 md:border-l'
                  : ''
              }`}
            >
              <FeatureIcon className='size-5 shrink-0' />
              <div>
                <h2 className='text-sm font-bold'>{String(title)}</h2>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {String(text)}
                </p>
              </div>
            </article>
          )
        })}
      </section>

      <section className='px-6 py-24 md:py-32'>
        <div className='mx-auto grid max-w-6xl gap-16 lg:grid-cols-[0.8fr_1.2fr]'>
          <div>
            <span className='inline-flex size-7 items-center justify-center bg-violet-500 font-mono text-xs font-bold text-white'>
              01
            </span>
            <h2 className='mt-6 text-4xl leading-[1.02] font-black tracking-[-0.045em] md:text-6xl'>
              {t('Lower price does not mean a single source.')}
            </h2>
          </div>
          <div className='self-end'>
            <div className='border-border grid border sm:grid-cols-3'>
              {[
                [t('Filter'), t('Remove unavailable routes')],
                [t('Prioritize'), t('Prefer the configured route')],
                [t('Retry'), t('Move to another available route')],
              ].map(([title, text], index) => (
                <div
                  key={title}
                  className={`p-6 ${
                    index > 0
                      ? 'border-border border-t sm:border-t-0 sm:border-l'
                      : ''
                  }`}
                >
                  <span className='font-mono text-xs text-violet-600 dark:text-violet-400'>
                    0{index + 1}
                  </span>
                  <h3 className='mt-4 font-bold'>{title}</h3>
                  <p className='text-muted-foreground mt-2 text-sm leading-6'>
                    {text}
                  </p>
                </div>
              ))}
            </div>
            <p className='text-muted-foreground mt-5 text-sm leading-7'>
              {t(
                'The homepage explains the routing strategy without exposing supplier names, procurement costs, or internal channel details.'
              )}
            </p>
          </div>
        </div>
      </section>

      <section className='bg-muted/35 border-border border-y px-6 py-24'>
        <div className='mx-auto max-w-6xl'>
          <div className='grid gap-10 lg:grid-cols-2 lg:items-end'>
            <div>
              <span className='inline-flex size-7 items-center justify-center bg-violet-500 font-mono text-xs font-bold text-white'>
                02
              </span>
              <h2 className='mt-6 text-4xl font-black tracking-tight md:text-5xl'>
                {t('Customer prices stay clear.')}
              </h2>
            </div>
            <div>
              <p className='text-muted-foreground max-w-xl leading-7'>
                {t(
                  'This template does not embed a stale model list. It sends customers to the built-in pricing page for current model metadata and configured customer prices.'
                )}
              </p>
              <Button
                variant='link'
                className='mt-3 h-auto px-0 font-semibold'
                render={<Link to='/pricing' />}
              >
                {t('Open current pricing')}
                <ArrowRight className='ml-2 size-4' />
              </Button>
            </div>
          </div>
          <div className='border-border bg-background mt-12 grid border md:grid-cols-3'>
            {[
              t('Configured customer price'),
              t('Current model availability'),
              t('Input and output billing details'),
            ].map((item) => (
              <div
                key={item}
                className='border-border flex items-center gap-3 border-b p-6 last:border-b-0 md:border-r md:border-b-0 md:last:border-r-0'
              >
                <Check className='size-5 text-violet-600 dark:text-violet-400' />
                <span className='text-sm font-semibold'>{item}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className='px-6 py-24 md:py-32'>
        <div className='mx-auto grid max-w-6xl gap-14 lg:grid-cols-[0.72fr_1.28fr] lg:items-center'>
          <div>
            <span className='inline-flex size-7 items-center justify-center bg-violet-500 font-mono text-xs font-bold text-white'>
              03
            </span>
            <h2 className='mt-6 text-4xl leading-tight font-black tracking-tight md:text-5xl'>
              {t('One contract. More choices.')}
            </h2>
            <p className='text-muted-foreground mt-5 leading-7'>
              {t(
                'Keep common client integrations while the gateway handles route selection behind the API.'
              )}
            </p>
          </div>
          <CodeExample />
        </div>
      </section>
    </main>
  )
}
