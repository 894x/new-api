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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { stringifyParameterCapabilityConfig } from '../../lib/parameter-capabilities'
import {
  applyParameterCapabilityTemplate,
  PARAMETER_CAPABILITY_TEMPLATES,
  PARAMETER_CAPABILITY_TEMPLATE_SOURCE,
  PARAMETER_CAPABILITY_TEMPLATE_VERSION,
  type ParameterCapabilityTemplate,
} from '../../lib/parameter-capability-templates'
import type { ParameterCapabilityConfig } from '../../types'

export function ParameterCapabilityTemplatePanel(props: {
  config: ParameterCapabilityConfig
  onApply: (config: ParameterCapabilityConfig) => void
}) {
  const { t } = useTranslation()
  const [template, setTemplate] = useState<ParameterCapabilityTemplate>(
    PARAMETER_CAPABILITY_TEMPLATES[0]
  )
  const [model, setModel] = useState<string>(template.model)
  const [replace, setReplace] = useState(false)
  const preview = applyParameterCapabilityTemplate(
    props.config,
    template,
    model,
    replace
  )
  return (
    <div className='flex h-full flex-col gap-4 overflow-auto p-5'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Copy a template into this channel, then edit it as needed. Future template updates do not change saved channels.'
        )}
      </p>
      <div className='flex flex-wrap gap-2'>
        {PARAMETER_CAPABILITY_TEMPLATES.map((item) => (
          <Button
            key={item.id}
            type='button'
            variant={template.id === item.id ? 'default' : 'outline'}
            aria-pressed={template.id === item.id}
            onClick={() => {
              setTemplate(item)
              setModel(item.model)
            }}
          >
            {item.name}
          </Button>
        ))}
      </div>
      <div className='space-y-2'>
        <Label htmlFor='capability-template-model'>
          {t('Upstream model name')}
        </Label>
        <Input
          id='capability-template-model'
          value={model}
          onChange={(event) => setModel(event.target.value)}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Use the exact model name after channel model mapping. Apply separately for each alias.'
          )}
        </p>
      </div>
      <Label className='flex items-center gap-2'>
        <input
          type='checkbox'
          checked={replace}
          onChange={(event) => setReplace(event.target.checked)}
        />
        {t('Replace existing constraints for template parameters')}
      </Label>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Existing effective constraints are kept by default. Review the resulting configuration below.'
        )}
      </p>
      <a
        href={PARAMETER_CAPABILITY_TEMPLATE_SOURCE}
        target='_blank'
        rel='noreferrer'
        className='text-primary text-sm underline'
      >
        {t('Documentation')} · {PARAMETER_CAPABILITY_TEMPLATE_VERSION}
      </a>
      <p className='text-muted-foreground text-xs'>
        {t(
          'These templates cover basic parameters only. Media limits and mode-specific combinations require separate validation.'
        )}
      </p>
      <pre
        aria-label={t('Template configuration preview')}
        className='bg-muted max-h-72 shrink-0 overflow-auto rounded-md p-3 text-xs'
      >
        {stringifyParameterCapabilityConfig(preview)}
      </pre>
      <Button
        className='self-start'
        type='button'
        disabled={!model.trim()}
        onClick={() => props.onApply(preview)}
      >
        {t('Apply template')}
      </Button>
    </div>
  )
}
