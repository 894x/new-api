import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { isUserDeleted } from '../constants'
import type { User } from '../types'

type CapturePolicy = { enabled: boolean; available: boolean }

function useUserRequestCapture(userId: number, enabled = true) {
  const client = useQueryClient()
  const queryKey = ['user-request-capture', userId]
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await api.get<{ data: CapturePolicy }>(
        `/api/user/${userId}/request-capture`
      )
      return response.data.data
    },
    enabled,
    staleTime: 0,
  })
  const mutation = useMutation({
    mutationFn: async (enabled: boolean) => {
      await api.put(`/api/user/${userId}/request-capture`, { enabled })
      return enabled
    },
    onSuccess: (enabled) => {
      client.setQueryData<CapturePolicy>(queryKey, (previous) =>
        previous ? { ...previous, enabled } : previous
      )
    },
  })

  return { query, mutation }
}

export function UserRequestCapture({ userId }: { userId: number }) {
  const { t } = useTranslation()
  const { query, mutation } = useUserRequestCapture(userId)

  return (
    <section className='space-y-2 rounded-md border p-3'>
      <div className='flex items-center justify-between gap-3'>
        <label htmlFor={`capture-${userId}`} className='text-sm font-medium'>
          {t('Save requests and responses')}
        </label>
        <Switch
          id={`capture-${userId}`}
          checked={query.data?.enabled ?? false}
          disabled={
            !query.data ||
            mutation.isPending ||
            (!query.data.available && !query.data.enabled)
          }
          onCheckedChange={(checked) => mutation.mutate(checked)}
        />
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Disabled by default. Changes apply immediately to new requests from this user. Captured content is visible only to administrators.'
        )}
      </p>
      {query.isPending && (
        <p role='status' className='text-xs'>
          {t('Loading...')}
        </p>
      )}
      {query.isError && (
        <div role='alert'>
          <p>{t('Failed to load capture settings')}</p>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )}
      {query.data && !query.data.available && (
        <p role='alert' className='text-xs'>
          {t('Capture storage is unavailable')}
        </p>
      )}
      {mutation.isError && (
        <p role='alert' className='text-xs'>
          {t('Failed to save capture settings')}
        </p>
      )}
    </section>
  )
}

export function UserRequestCaptureCell({ user }: { user: User }) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canManage =
    currentUser?.role === ROLE.SUPER_ADMIN ||
    (currentUser?.role ?? 0) > user.role
  const shouldLoad = canManage && !isUserDeleted(user)
  const { query, mutation } = useUserRequestCapture(user.id, shouldLoad)

  if (!shouldLoad) {
    return <span className='text-muted-foreground text-sm'>—</span>
  }

  const policy = query.data
  const disabled =
    !policy || mutation.isPending || (!policy.available && !policy.enabled)
  let tooltip = t('Save requests and responses')
  if (query.isError) {
    tooltip = t('Failed to load capture settings')
  } else if (policy && !policy.available && !policy.enabled) {
    tooltip = t('Capture storage is unavailable')
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className='inline-flex'>
            <Switch
              size='sm'
              checked={policy?.enabled ?? false}
              disabled={disabled}
              aria-label={`${t('Save requests and responses')}: ${user.username}`}
              onCheckedChange={(checked) =>
                mutation.mutate(checked, {
                  onError: (error) =>
                    handleServerError(
                      error,
                      t('Failed to save capture settings')
                    ),
                })
              }
            />
          </span>
        }
      />
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  )
}
