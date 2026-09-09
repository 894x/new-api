import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

type CapturePart = {
  stage: string
  attempt?: number
  channel_id?: number
  model?: string
  content_type?: string
  status_code?: number
  upstream_request_id?: string
  bytes: number
  complete: boolean
  truncated: boolean
  redacted: boolean
  omitted: boolean
  body: string
  tail?: string
}
type CaptureData = {
  status: string
  record?: { reason?: string; raw_bytes: number; stored_bytes: number }
  parts?: CapturePart[]
}

export function RequestCapture({ requestId }: { requestId: string }) {
  const { t } = useTranslation()
  const [body, setBody] = useState<CaptureData>()
  const [loading, setLoading] = useState(false)
  const [failed, setFailed] = useState(false)
  const [copyFailed, setCopyFailed] = useState(false)
  const query = useQuery({
    queryKey: ['request-capture-status', requestId],
    queryFn: async () =>
      (
        await api.get<{ data: CaptureData }>(
          `/api/log/request-capture/${encodeURIComponent(requestId)}`
        )
      ).data.data,
    staleTime: 0,
  })
  const status = body?.status ?? query.data?.status
  const statusLabels: Record<string, string> = {
    not_captured: t('Not captured'),
    recording: t('Capture in progress'),
    ready: t('Captured'),
    partial: t('Partially captured'),
    failed: t('Capture failed'),
    expired: t('Capture expired'),
    unavailable: t('Capture storage is unavailable'),
  }
  const stageLabels: Record<string, string> = {
    client_request: t('Client request'),
    upstream_request: t('Upstream request'),
    upstream_response: t('Upstream response'),
    client_response: t('Client response'),
  }
  async function loadBody() {
    setLoading(true)
    setFailed(false)
    try {
      const response = await api.get<{ data: CaptureData }>(
        `/api/log/request-capture/${encodeURIComponent(requestId)}`,
        { params: { body: true } }
      )
      setBody(response.data.data)
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }
  function download() {
    if (!body?.parts) return
    const blob = new Blob(
      [`${body.parts.map((part) => JSON.stringify(part)).join('\n')}\n`],
      { type: 'application/x-ndjson' }
    )
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `capture-${requestId.replaceAll(/[^a-zA-Z0-9_-]/g, '_')}.jsonl`
    link.click()
    URL.revokeObjectURL(url)
  }
  return (
    <section className='min-w-0 space-y-2 rounded-md border p-3'>
      <h3 className='text-sm font-semibold'>{t('Requests and responses')}</h3>
      <p role='status' className='text-xs'>
        {query.isPending
          ? t('Loading...')
          : (statusLabels[status ?? ''] ?? t('Unknown'))}
      </p>
      {query.isError && <p role='alert'>{t('Failed to load capture')}</p>}
      {(query.isError || status === 'recording') && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      )}
      {!body?.parts &&
        (status === 'ready' ||
          status === 'partial' ||
          status === 'unavailable') && (
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={loading}
            onClick={() => void loadBody()}
          >
            {loading ? t('Loading...') : t('View captured content')}
          </Button>
        )}
      {failed && <p role='alert'>{t('Failed to load capture')}</p>}
      {body?.parts && (
        <>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Captured content may be incomplete or redacted. Response tails are separate fragments after truncation.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={download}
            >
              {t('Download')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(
                    JSON.stringify(body.parts, null, 2)
                  )
                  setCopyFailed(false)
                } catch {
                  setCopyFailed(true)
                }
              }}
            >
              {t('Copy to clipboard')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setBody(undefined)}
            >
              {t('Hide')}
            </Button>
          </div>
          {copyFailed && <p role='alert'>{t('Copy failed')}</p>}
          {body.parts.map((part, index) => (
            <details
              key={`${part.stage}-${part.attempt ?? index}`}
              className='min-w-0 rounded border p-2'
            >
              <summary className='cursor-pointer text-xs'>
                {stageLabels[part.stage] ?? part.stage}
                {part.attempt ? ` · ${t('Attempt')} ${part.attempt}` : ''} ·{' '}
                {part.bytes} B
              </summary>
              <dl className='my-2 grid grid-cols-2 gap-1 text-xs'>
                <dt>{t('Channel')}</dt>
                <dd>{part.channel_id ?? '-'}</dd>
                <dt>{t('Model')}</dt>
                <dd className='break-all'>{part.model ?? '-'}</dd>
                <dt>{t('Status Code')}</dt>
                <dd>{part.status_code ?? '-'}</dd>
                <dt>{t('Upstream Request ID')}</dt>
                <dd className='break-all'>{part.upstream_request_id ?? '-'}</dd>
              </dl>
              <p className='text-muted-foreground text-xs'>
                {[
                  !part.complete && t('Incomplete'),
                  part.truncated && t('Truncated'),
                  part.redacted && t('Redacted'),
                  part.omitted && t('Content omitted'),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </p>
              <pre className='mt-2 max-h-80 overflow-auto text-xs break-all whitespace-pre-wrap'>
                {part.body || t('No content')}
              </pre>
              {part.tail && (
                <>
                  <p className='mt-2 text-xs font-medium'>
                    {t('Response tail')}
                  </p>
                  <pre className='max-h-60 overflow-auto text-xs break-all whitespace-pre-wrap'>
                    {part.tail}
                  </pre>
                </>
              )}
            </details>
          ))}
        </>
      )}
    </section>
  )
}
