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
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

type MonitorTarget = { model: string; endpoint_type: string }
type MonitorConfig = { enabled?: boolean; targets?: MonitorTarget[] }
const protocols = [
  { value: 'auto', label: 'Automatic' },
  { value: 'openai', label: 'OpenAI Chat' },
  { value: 'openai-response', label: 'OpenAI Responses' },
  { value: 'anthropic', label: 'Anthropic Messages' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'embeddings', label: 'Embeddings' },
  { value: 'jina-rerank', label: 'Rerank' },
]

function parseMonitorConfig(value: string): MonitorConfig {
  if (!value) return {}
  try {
    const config: unknown = JSON.parse(value)
    if (config && typeof config === 'object' && !Array.isArray(config)) {
      const parsed = config as MonitorConfig
      if (
        (parsed.enabled === undefined || typeof parsed.enabled === 'boolean') &&
        (parsed.targets === undefined ||
          (Array.isArray(parsed.targets) &&
            parsed.targets.every(
              (target) =>
                target &&
                typeof target.model === 'string' &&
                typeof target.endpoint_type === 'string'
            )))
      ) {
        return parsed
      }
    }
  } catch {
    /* Invalid persisted data is surfaced by server validation. */
  }
  return {}
}

export function ChannelMonitorConfigEditor(props: {
  value: string
  models: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const config = parseMonitorConfig(props.value)
  const enabled = config.enabled !== false
  const targets = Array.isArray(config.targets) ? config.targets : []
  const models = props.models
    .split(',')
    .map((model) => model.trim())
    .filter(Boolean)
  const availableTarget = protocols
    .flatMap((protocol) =>
      models.map((model) => ({
        model,
        endpoint_type: protocol.value === 'auto' ? '' : protocol.value,
      }))
    )
    .find(
      (candidate) =>
        !targets.some(
          (target) =>
            target.model === candidate.model &&
            target.endpoint_type === candidate.endpoint_type
        )
    )
  function update(patch: Partial<MonitorConfig>) {
    props.onChange(JSON.stringify({ ...config, ...patch }))
  }
  return (
    <div
      className='@container/monitor-config space-y-3 rounded-lg border p-3'
      role='group'
      aria-label={t('Channel monitoring')}
    >
      <div className='flex items-center justify-between gap-3'>
        <div className='space-y-1'>
          <p className='text-sm font-medium'>{t('Channel monitoring')}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Register this channel for continuous probes and show its health in model routing.'
            )}
          </p>
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={(checked) => update({ enabled: checked })}
          disabled={props.disabled}
          aria-label={t('Enable channel monitoring')}
        />
      </div>
      {enabled && (
        <>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Choose model and protocol targets. An empty list uses the test model or the first channel model.'
            )}
          </p>
          {targets.map((target, index) => (
            <div
              className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 @min-[26rem]/monitor-config:grid-cols-[minmax(0,1fr)_10rem_auto]'
              key={`${target.model}:${target.endpoint_type}`}
            >
              <Combobox
                className='col-span-2 min-w-0 @min-[26rem]/monitor-config:col-span-1'
                options={models
                  .filter(
                    (model) =>
                      !targets.some(
                        (item, i) =>
                          i !== index &&
                          item.model === model &&
                          item.endpoint_type === target.endpoint_type
                      )
                  )
                  .map((model) => ({ value: model, label: model }))}
                value={target.model}
                onValueChange={(model) => {
                  if (model) {
                    update({
                      targets: targets.map((item, i) =>
                        i === index ? { ...item, model } : item
                      ),
                    })
                  }
                }}
                disabled={props.disabled}
                aria-label={t('Monitoring model {{index}}', {
                  index: index + 1,
                })}
                placeholder={t('Select model')}
              />
              <Select
                value={target.endpoint_type || 'auto'}
                onValueChange={(value) => {
                  if (value) {
                    update({
                      targets: targets.map((item, i) =>
                        i === index
                          ? {
                              ...item,
                              endpoint_type: value === 'auto' ? '' : value,
                            }
                          : item
                      ),
                    })
                  }
                }}
                disabled={props.disabled}
              >
                <SelectTrigger
                  className='w-full @min-[26rem]/monitor-config:w-40'
                  aria-label={t('Monitoring protocol {{index}}', {
                    index: index + 1,
                  })}
                >
                  <SelectValue>
                    {(value: string) =>
                      t(
                        protocols.find((item) => item.value === value)?.label ??
                          'Automatic'
                      )
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {protocols.map((item) => (
                    <SelectItem
                      key={item.value}
                      value={item.value}
                      disabled={targets.some(
                        (other, i) =>
                          i !== index &&
                          other.model === target.model &&
                          other.endpoint_type ===
                            (item.value === 'auto' ? '' : item.value)
                      )}
                    >
                      {t(item.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type='button'
                variant='ghost'
                size='icon-sm'
                disabled={props.disabled}
                aria-label={t('Remove monitoring target {{index}}', {
                  index: index + 1,
                })}
                onClick={() =>
                  update({ targets: targets.filter((_, i) => i !== index) })
                }
              >
                <Trash2 className='size-4' />
              </Button>
            </div>
          ))}
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={
              props.disabled || !availableTarget || targets.length >= 16
            }
            onClick={() => {
              if (availableTarget) {
                update({ targets: [...targets, availableTarget] })
              }
            }}
          >
            <Plus className='size-4' />
            {t('Add monitoring target')}
          </Button>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Probe request settings are managed in System Settings → Request Policies → Channel monitoring.'
            )}
          </p>
        </>
      )}
    </div>
  )
}
