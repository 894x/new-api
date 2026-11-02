import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { LogExportRequest } from './types'

export interface LogExportConfig {
  max_rows: number
  retention_days: number
  max_file_bytes: number
}

export interface LogExportRecord {
  task_id: string
  view: LogExportRequest['view']
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  error: string
  max_rows: number
  row_count: number
  file_size: number
  filename: string
  created_at: number
  updated_at: number
  expires_at: number
  filters: LogExportRequest
}

export interface LogExportHistory {
  data: LogExportRecord[]
  total: number
}

export interface LogExportFileDestination {
  createWritable: () => Promise<WritableStream<Uint8Array>>
}

export async function getLogExportConfig(): Promise<LogExportConfig> {
  const response = await api.get('/api/log/export/config')
  return requireServerSuccess(response.data).data
}

export function logExportEndpoint(scope: 'all' | 'self'): string {
  return scope === 'self' ? '/api/log/self/exports' : '/api/log/exports'
}

export async function createLogExport(
  request: LogExportRequest
): Promise<LogExportRecord> {
  const endpoint = logExportEndpoint(request.view === 'self' ? 'self' : 'all')
  const response = await api.post(endpoint, request)
  return requireServerSuccess(response.data).data
}

export async function getLogExportHistory(
  scope: 'all' | 'self',
  offset: number
): Promise<LogExportHistory> {
  const response = await api.get(logExportEndpoint(scope), {
    params: { offset },
  })
  const result = requireServerSuccess(response.data)
  return { data: result.data ?? [], total: result.total }
}

export async function downloadLogExport(
  record: LogExportRecord,
  scope: 'all' | 'self',
  destination?: Promise<LogExportFileDestination>
): Promise<{ blob: Blob; filename: string } | { saved: true }> {
  const endpoint = `${logExportEndpoint(scope)}/${encodeURIComponent(record.task_id)}/file`
  if (destination) {
    const file = await destination
    const response = await api.get<ReadableStream<Uint8Array>>(endpoint, {
      adapter: 'fetch',
      responseType: 'stream',
      skipBusinessError: true,
      skipErrorHandler: true,
    })
    if (
      String(response.headers['content-type'] ?? '').includes(
        'application/json'
      )
    ) {
      requireServerSuccess(await new Response(response.data).json())
      throw new Error('Failed to download export')
    }
    await response.data.pipeTo(await file.createWritable())
    return { saved: true }
  }
  // Downloads need the same Authorization/refresh path as every dashboard API.
  const response = await api.get(endpoint, {
    responseType: 'blob',
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  if (
    String(response.headers['content-type'] ?? '').includes('application/json')
  ) {
    const result = JSON.parse(await (response.data as Blob).text())
    requireServerSuccess(result)
    throw new Error('Failed to download export')
  }
  return { blob: response.data as Blob, filename: record.filename }
}
