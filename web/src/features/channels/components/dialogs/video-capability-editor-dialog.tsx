/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Plus, Trash2, Video } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import {
  parseVideoCapabilityConfig,
  stringifyVideoCapabilityConfig,
  validateVideoCapabilityConfig,
} from '../../lib/video-capabilities'
import type { VideoCapabilityConfig } from '../../types'

export interface VideoCapabilityEditorDialogProps {
  open: boolean
  value: string
  models: string[]
  onOpenChange: (open: boolean) => void
  onSave: (value: string) => void
}

interface VideoCapabilityRow {
  id: number
  model: string
  resolutions: string
}

export function VideoCapabilityEditorDialog(
  props: VideoCapabilityEditorDialogProps
) {
  if (!props.open) return null
  return (
    <VideoCapabilityEditorSession
      key={props.value}
      value={props.value}
      models={props.models}
      onOpenChange={props.onOpenChange}
      onSave={props.onSave}
    />
  )
}

function VideoCapabilityEditorSession(
  props: Omit<VideoCapabilityEditorDialogProps, 'open'>
) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<VideoCapabilityRow[]>(() =>
    Object.entries(parseVideoCapabilityConfig(props.value).models || {}).map(
      ([model, capability], index) => ({
        id: index + 1,
        model,
        resolutions: (capability.resolutions || []).join(', '),
      })
    )
  )

  const config: VideoCapabilityConfig = {
    models: Object.fromEntries(
      rows.map((row) => [
        row.model.trim(),
        {
          resolutions: row.resolutions
            .split(',')
            .map((value) => value.trim())
            .filter(Boolean),
        },
      ])
    ),
  }
  const duplicateModels = rows
    .map((row) => row.model.trim())
    .filter((model, index, models) => model && models.indexOf(model) !== index)
  const errors = validateVideoCapabilityConfig(config)
  if (duplicateModels.length > 0) {
    errors.push({ model: duplicateModels[0], message: 'Duplicate model rule.' })
  }

  function addRow(): void {
    setRows((current) => {
      const usedModels = new Set(current.map((row) => row.model))
      const model = props.models.find((item) => !usedModels.has(item)) || ''
      const nextId = current.reduce((max, row) => Math.max(max, row.id), 0) + 1
      return [...current, { id: nextId, model, resolutions: '720p' }]
    })
  }

  function updateRow(id: number, patch: Partial<VideoCapabilityRow>): void {
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, ...patch } : row))
    )
  }

  function removeRow(id: number): void {
    setRows((current) => current.filter((row) => row.id !== id))
  }

  function handleSave(): void {
    if (errors.length > 0) {
      toast.error(t('Fix video capability errors before saving.'))
      return
    }
    props.onSave(stringifyVideoCapabilityConfig(config))
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open
      onOpenChange={props.onOpenChange}
      title={t('Video Resolution Capabilities')}
      description={t(
        'Limit each public video model to the resolutions this channel supports. Models without a rule remain unrestricted.'
      )}
      contentHeight='min(64vh, 640px)'
      contentClassName='sm:max-w-4xl'
      footer={
        <>
          <Badge
            className='mr-auto'
            variant={errors.length > 0 ? 'destructive' : 'secondary'}
          >
            {errors.length > 0
              ? t('{{count}} configuration error(s)', { count: errors.length })
              : t('{{count}} video model rule(s)', { count: rows.length })}
          </Badge>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSave}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <Alert>
          <Video className='h-4 w-4' />
          <AlertDescription>
            {t(
              'This filter is evaluated only for video submission requests. Chat, image, audio, and other channel traffic are unchanged.'
            )}
          </AlertDescription>
        </Alert>

        {errors.length > 0 && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t(errors[0].message, errors[0].values)}
            </AlertDescription>
          </Alert>
        )}

        <div className='space-y-3'>
          {rows.map((row) => (
            <div
              key={row.id}
              className='border-border/60 grid gap-3 rounded-lg border p-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end'
            >
              <Field>
                <FieldLabel>{t('Public video model')}</FieldLabel>
                <Combobox
                  options={props.models.map((model) => ({
                    value: model,
                    label: model,
                  }))}
                  value={row.model}
                  onValueChange={(value) =>
                    updateRow(row.id, { model: value || '' })
                  }
                  allowCustomValue
                  openOnFocus
                  showAllOptionsOnFocus
                  placeholder={t('Model name')}
                  className='w-full'
                />
              </Field>
              <Field>
                <FieldLabel>{t('Supported resolutions')}</FieldLabel>
                <Input
                  value={row.resolutions}
                  onChange={(event) =>
                    updateRow(row.id, { resolutions: event.target.value })
                  }
                  placeholder={t('720p, 1080p, 4k')}
                />
                <FieldDescription>
                  {t('Comma-separated labels or dimensions.')}
                </FieldDescription>
              </Field>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Delete video model rule')}
                onClick={() => removeRow(row.id)}
              >
                <Trash2 className='h-4 w-4' />
              </Button>
            </div>
          ))}
        </div>

        <Button type='button' variant='outline' onClick={addRow}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add video model rule')}
        </Button>
      </div>
    </Dialog>
  )
}
