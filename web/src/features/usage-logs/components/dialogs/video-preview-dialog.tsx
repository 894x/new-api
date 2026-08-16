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
import { ExternalLink, Video } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { tryPrettyJson } from '@/lib/utils'

interface VideoPreviewDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  sourceUrl: string
  rawData?: unknown
}

function formatRawData(data: unknown): string {
  if (data === undefined || data === '') return ''
  if (typeof data === 'string') return tryPrettyJson(data)

  try {
    return JSON.stringify(data, null, 2) ?? String(data)
  } catch {
    return String(data)
  }
}

export function VideoPreviewDialog(props: VideoPreviewDialogProps) {
  const { t } = useTranslation()
  const rawJson = useMemo(
    () => (props.open ? formatRawData(props.rawData) : ''),
    [props.open, props.rawData]
  )
  const footer = (
    <Button
      variant='outline'
      render={
        <a href={props.sourceUrl} target='_blank' rel='noopener noreferrer' />
      }
    >
      <ExternalLink />
      {t('Open in new tab')}
    </Button>
  )
  const videoPreview = (
    <video
      src={props.sourceUrl}
      controls
      preload='metadata'
      className='max-h-[65vh] w-full rounded-lg bg-black'
    />
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <IconBadge tone='chart-2' size='sm'>
            <Video />
          </IconBadge>
          {t('Video')}
        </>
      }
      titleClassName='flex items-center gap-2'
      contentClassName='sm:max-w-4xl'
      contentHeight='auto'
      footer={footer}
    >
      {rawJson ? (
        <Tabs defaultValue='preview'>
          <TabsList className='grid w-full grid-cols-2'>
            <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
            <TabsTrigger value='raw-json'>{t('Raw JSON')}</TabsTrigger>
          </TabsList>
          <TabsContent value='preview' className='mt-2'>
            {videoPreview}
          </TabsContent>
          <TabsContent value='raw-json' className='relative mt-2'>
            <CopyButton
              value={rawJson}
              className='absolute top-2 right-2 z-10 size-8'
              iconClassName='size-4'
              tooltip={t('Copy')}
              successTooltip={t('Copied')}
              aria-label={t('Copy')}
            />
            <pre
              aria-label={t('Raw JSON')}
              className='bg-muted/50 max-h-[60vh] overflow-auto rounded-md border p-4 pr-12 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'
            >
              {rawJson}
            </pre>
          </TabsContent>
        </Tabs>
      ) : (
        videoPreview
      )}
    </Dialog>
  )
}
