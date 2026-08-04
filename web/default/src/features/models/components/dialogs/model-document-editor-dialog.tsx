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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import {
  deleteModelDocumentOverride,
  getModelDocumentEditor,
  previewModelDocument,
  publishModelDocument,
  saveModelDocumentDraft,
} from '../../api'
import { modelsQueryKeys } from '../../lib/query-keys'
import type {
  Model,
  ModelDocumentEditor,
  ModelDocumentEditorPayload,
} from '../../types'

type ModelDocumentEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: Model | null
}

const EMPTY_DOCUMENT: ModelDocumentEditorPayload = {
  slug: '',
  title: '',
  vendor: '',
  category: '',
  summary: '',
  html: '',
}

export function ModelDocumentEditorDialog(
  props: ModelDocumentEditorDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<ModelDocumentEditorPayload>(EMPTY_DOCUMENT)
  const [previewHTML, setPreviewHTML] = useState('')
  const [previewPending, setPreviewPending] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const modelId = props.model?.id ?? 0

  const documentQuery = useQuery({
    queryKey: ['model-document-editor', modelId],
    queryFn: async () => {
      const response = await getModelDocumentEditor(modelId)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load model document'))
      }
      return response.data
    },
    enabled: props.open && modelId > 0,
  })

  useEffect(() => {
    const document = documentQuery.data
    if (!props.open || modelId <= 0 || !document) return
    const nextForm: ModelDocumentEditorPayload = {
      slug: document.slug,
      title: document.title,
      vendor: document.vendor,
      category: document.category,
      summary: document.summary,
      html: document.html,
    }
    setForm(nextForm)
    setPreviewHTML(document.html)
    setPreviewPending(true)
    let cancelled = false
    void previewModelDocument(modelId, document.html)
      .then((response) => {
        if (cancelled) return
        if (response.success && response.data) {
          setPreviewHTML(response.data)
        }
      })
      .finally(() => {
        if (!cancelled) setPreviewPending(false)
      })
    return () => {
      cancelled = true
    }
  }, [documentQuery.data, modelId, props.open])

  const savedForm = useMemo<ModelDocumentEditorPayload | null>(() => {
    const document = documentQuery.data
    if (!document) return null
    return {
      slug: document.slug,
      title: document.title,
      vendor: document.vendor,
      category: document.category,
      summary: document.summary,
      html: document.html,
    }
  }, [documentQuery.data])

  const isDirty = savedForm
    ? form.slug !== savedForm.slug ||
      form.title !== savedForm.title ||
      form.vendor !== savedForm.vendor ||
      form.category !== savedForm.category ||
      form.summary !== savedForm.summary ||
      form.html !== savedForm.html
    : false
  const htmlBytes = useMemo(
    () => new TextEncoder().encode(form.html).length,
    [form.html]
  )

  const updateCachedDocument = (document: ModelDocumentEditor) => {
    queryClient.setQueryData(['model-document-editor', modelId], document)
    void queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
    if (modelId > 0) {
      void queryClient.invalidateQueries({
        queryKey: modelsQueryKeys.detail(modelId),
      })
    }
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const response = await saveModelDocumentDraft(modelId, form)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to save document draft'))
      }
      return response.data
    },
    onSuccess: (document) => {
      updateCachedDocument(document)
      toast.success(t('Document draft saved'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const publishMutation = useMutation({
    mutationFn: async () => {
      const saveResponse = await saveModelDocumentDraft(modelId, form)
      if (!saveResponse.success) {
        throw new Error(
          saveResponse.message || t('Failed to save document draft')
        )
      }
      const publishResponse = await publishModelDocument(modelId)
      if (!publishResponse.success || !publishResponse.data) {
        throw new Error(
          publishResponse.message || t('Failed to publish model document')
        )
      }
      return publishResponse.data
    },
    onSuccess: (document) => {
      updateCachedDocument(document)
      toast.success(t('Model document published'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: async () => {
      const response = await deleteModelDocumentOverride(modelId)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to restore model document')
        )
      }
      return response.data
    },
    onSuccess: (document) => {
      updateCachedDocument(document)
      setDeleteConfirmOpen(false)
      toast.success(
        document.has_builtin
          ? t('Built-in model document restored')
          : t('Custom model document deleted')
      )
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const handlePreview = async () => {
    if (modelId <= 0) return
    setPreviewPending(true)
    try {
      const response = await previewModelDocument(modelId, form.html)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to refresh preview'))
      }
      setPreviewHTML(response.data)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to refresh preview')
      )
    } finally {
      setPreviewPending(false)
    }
  }

  const effectiveSource = documentQuery.data?.effective_source
  let effectiveSourceLabel = ''
  if (effectiveSource === 'custom') {
    effectiveSourceLabel = t('Online document is live')
  } else if (effectiveSource === 'builtin') {
    effectiveSourceLabel = t('Built-in document is live')
  } else if (effectiveSource === 'none') {
    effectiveSourceLabel = t('No document is live')
  }
  const editorReady = documentQuery.isSuccess && modelId > 0
  const isBusy =
    !editorReady || saveMutation.isPending || publishMutation.isPending

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='grid h-[calc(100vh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-[min(1440px,calc(100vw-2rem))]'>
          <DialogHeader className='pr-10'>
            <div className='flex flex-wrap items-center gap-2'>
              <DialogTitle>{t('Edit model document')}</DialogTitle>
              {effectiveSource && (
                <Badge variant='secondary'>{effectiveSourceLabel}</Badge>
              )}
              {isDirty && <Badge variant='outline'>{t('Unsaved')}</Badge>}
            </div>
            <DialogDescription>
              {props.model?.model_name || ''} ·{' '}
              {t('Paste a complete HTML document or an HTML fragment.')}
            </DialogDescription>
          </DialogHeader>

          {documentQuery.isLoading && (
            <div className='flex items-center justify-center'>
              <Spinner className='size-6' />
            </div>
          )}
          {documentQuery.isError && (
            <div className='text-destructive flex items-center justify-center'>
              {documentQuery.error.message}
            </div>
          )}
          {documentQuery.isSuccess && (
            <div className='grid min-h-0 gap-4 overflow-y-auto lg:grid-cols-[minmax(420px,0.9fr)_minmax(480px,1.1fr)] lg:overflow-hidden'>
              <div className='flex min-h-0 flex-col gap-4 lg:overflow-y-auto lg:pr-1'>
                <FieldGroup className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                  <Field>
                    <FieldLabel htmlFor='model-document-title'>
                      {t('Document title')}
                    </FieldLabel>
                    <Input
                      id='model-document-title'
                      value={form.title}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          title: event.target.value,
                        }))
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='model-document-slug'>Slug</FieldLabel>
                    <Input
                      id='model-document-slug'
                      value={form.slug}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          slug: event.target.value,
                        }))
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='model-document-vendor'>
                      {t('Provider')}
                    </FieldLabel>
                    <Input
                      id='model-document-vendor'
                      value={form.vendor}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          vendor: event.target.value,
                        }))
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='model-document-category'>
                      {t('Model category')}
                    </FieldLabel>
                    <Input
                      id='model-document-category'
                      value={form.category}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          category: event.target.value,
                        }))
                      }
                    />
                  </Field>
                </FieldGroup>
                <Field>
                  <FieldLabel htmlFor='model-document-summary'>
                    {t('Document summary')}
                  </FieldLabel>
                  <Input
                    id='model-document-summary'
                    value={form.summary}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        summary: event.target.value,
                      }))
                    }
                  />
                </Field>
                <Field className='min-h-[360px] flex-1'>
                  <div className='flex items-center justify-between gap-3'>
                    <FieldLabel htmlFor='model-document-html'>
                      {t('HTML source')}
                    </FieldLabel>
                    <span className='text-muted-foreground text-xs'>
                      {htmlBytes.toLocaleString()} / 1,048,576 bytes
                    </span>
                  </div>
                  <Textarea
                    id='model-document-html'
                    value={form.html}
                    spellCheck={false}
                    className='max-h-none min-h-[320px] flex-1 resize-none font-mono text-xs leading-5'
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        html: event.target.value,
                      }))
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Scripts and form submissions are blocked when the document is displayed.'
                    )}
                  </FieldDescription>
                </Field>
              </div>

              <section className='bg-background flex min-h-[440px] min-w-0 flex-col overflow-hidden rounded-lg border lg:min-h-0'>
                <div className='bg-muted/40 flex h-11 shrink-0 items-center justify-between border-b px-3'>
                  <span className='font-medium'>{t('Document preview')}</span>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={previewPending || !form.html.trim()}
                    onClick={() => void handlePreview()}
                  >
                    {previewPending ? (
                      <Spinner data-icon='inline-start' />
                    ) : (
                      <RefreshCw data-icon='inline-start' />
                    )}
                    {t('Refresh preview')}
                  </Button>
                </div>
                <iframe
                  title={t('Model document preview')}
                  sandbox='allow-same-origin'
                  srcDoc={previewHTML}
                  className='bg-background size-full min-h-[400px] border-0'
                />
              </section>
            </div>
          )}

          <DialogFooter className='items-center sm:justify-between'>
            <div className='flex flex-1 items-center'>
              {documentQuery.data?.has_custom && (
                <Button
                  type='button'
                  variant='destructive'
                  disabled={deleteMutation.isPending || isBusy}
                  onClick={() => setDeleteConfirmOpen(true)}
                >
                  {documentQuery.data.has_builtin
                    ? t('Restore built-in document')
                    : t('Delete custom document')}
                </Button>
              )}
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={isBusy || !form.html.trim()}
              onClick={() => saveMutation.mutate()}
            >
              {saveMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Save draft')}
            </Button>
            <Button
              type='button'
              disabled={isBusy || !form.html.trim()}
              onClick={() => publishMutation.mutate()}
            >
              {publishMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Publish document')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={
          documentQuery.data?.has_builtin
            ? t('Restore built-in document')
            : t('Delete custom document')
        }
        desc={
          documentQuery.data?.has_builtin
            ? t(
                'The online version and its draft will be deleted. The built-in document will become active again.'
              )
            : t('The online version and its draft will be permanently deleted.')
        }
        confirmText={
          documentQuery.data?.has_builtin ? t('Restore') : t('Delete')
        }
        destructive
        handleConfirm={() => deleteMutation.mutate()}
      />
    </>
  )
}
