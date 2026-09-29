import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type {
  User,
  UserChannelRoutingConfig,
  UserGroupConfig,
} from '../../types'
import { UserGroupConfigDrawer } from '../user-group-config-drawer'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider, useUsers } from '../users-provider'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), patch: vi.fn(), put: vi.fn(), post: vi.fn() },
}))

const user: User = {
  id: 42,
  username: 'acme',
  display_name: 'Acme',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
}

const sharedConfig: UserGroupConfig = {
  user_id: 42,
  username: 'acme',
  group: 'default',
  dedicated_group_name: '企业客户-acme',
  is_dedicated_group: false,
  revision: 'revision-1',
  group_user_count: 2,
  discounts: { default: 0.8 },
  rate_limit_enabled: true,
  global_rate_limit_enabled: true,
  rate_limit: { limits: [10, 8, 1000], models: {} },
  model_channel_groups: { 'model-x': ['pool-a'] },
  available_group_ratios: { default: 1 },
}

const routingConfig: UserChannelRoutingConfig = {
  model: 'model-x',
  models: [],
  revision: 'routing-revision-1',
  channels: [
    {
      channel_id: 11,
      channel_name: 'Pool Alpha',
      default_priority: 100,
      model_priority: null,
      inherited_priority: 100,
      priority_override: null,
      effective_priority: 100,
      enabled: true,
      available: true,
    },
  ],
}

function renderGroupConfig() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UserGroupConfigDrawer open onOpenChange={vi.fn()} user={user} />
      </UsersProvider>
    </QueryClientProvider>
  )
}

function CurrentDialog() {
  const { open } = useUsers()
  return <output data-testid='current-dialog'>{open}</output>
}

function renderUserEditor() {
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/user/42') {
      return { data: { success: true, data: user } }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/authz/catalog') {
      return { data: { success: true, data: { resources: [], roles: [] } } }
    }
    return {
      data: { success: true, data: { enabled: false, available: true } },
    }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <CurrentDialog />
        <UsersMutateDrawer open onOpenChange={vi.fn()} currentRow={user} />
      </UsersProvider>
    </QueryClientProvider>
  )
}

describe('User group quick configuration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url === '/api/user/42/channel-routing-overrides') {
        return { data: { success: true, data: routingConfig } }
      }
      return { data: { success: true, data: sharedConfig } }
    })
  })

  it('shows shared-group impact and copies policies into a named dedicated group', async () => {
    const dedicatedConfig = {
      ...sharedConfig,
      group: '企业客户-acme',
      is_dedicated_group: true,
      revision: 'revision-2',
      group_user_count: 1,
    }
    vi.mocked(api.post).mockResolvedValue({
      data: { success: true, data: dedicatedConfig },
    })

    renderGroupConfig()

    expect(
      await screen.findByText(/Group default is used by 2 user/)
    ).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Create dedicated group' })
    )
    const confirmation = screen.getByRole('alertdialog')
    expect(within(confirmation).getByText(/企业客户-acme/)).toBeInTheDocument()
    fireEvent.click(
      within(confirmation).getByRole('button', {
        name: 'Create dedicated group',
      })
    )

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/user/42/group-config/dedicated',
        { group: 'default', revision: 'revision-1' }
      )
    )
    expect(
      await screen.findByText(/Group 企业客户-acme is used by 1 user/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Create dedicated group' })
    ).not.toBeInTheDocument()
  })

  it('leaves the existing group unchanged when creation is cancelled', async () => {
    renderGroupConfig()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Create dedicated group' })
    )
    const confirmation = screen.getByRole('alertdialog')
    fireEvent.click(
      within(confirmation).getByRole('button', { name: 'Cancel' })
    )

    expect(api.post).not.toHaveBeenCalled()
    expect(
      screen.getByText(/Group default is used by 2 user/)
    ).toBeInTheDocument()
  })

  it('warns when a selected model pool has no enabled channel candidates', async () => {
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: { 'model-x': { candidate_count: 0 } },
      },
    })
    renderGroupConfig()

    fireEvent.click(await screen.findByRole('tab', { name: 'Channels' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Preview channel availability' })
    )

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/user/42/group-config/preview',
        {
          group: 'default',
          model_channel_groups: { 'model-x': ['pool-a'] },
        }
      )
    )
    expect(
      await screen.findByText(/No enabled channels match this model/)
    ).toBeInTheDocument()
  })

  it('shows user priorities inline only for channels matched by the selected pools', async () => {
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: { 'model-x': { candidate_count: 1 } },
      },
    })
    renderGroupConfig()

    fireEvent.click(await screen.findByRole('tab', { name: 'Channels' }))
    expect(screen.queryByText('Pool Alpha #11')).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Preview channel availability' })
    )

    expect(await screen.findByText('Pool Alpha #11')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      {
        params: {
          model: 'model-x',
          channel_groups: JSON.stringify(['pool-a']),
        },
      }
    )
  })

  it('opens group configuration from the user editor without changing the user', async () => {
    renderUserEditor()

    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/42'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure group policies' })
    )

    expect(screen.getByTestId('current-dialog')).toHaveTextContent(
      'group-config'
    )
    expect(api.put).not.toHaveBeenCalled()
  })

  it('saves edited user fields before opening group configuration', async () => {
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
    renderUserEditor()

    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/42'))
    fireEvent.change(screen.getByRole('textbox', { name: 'Remark' }), {
      target: { value: 'Enterprise account' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Save and configure group policies' })
    )

    await waitFor(() => expect(api.put).toHaveBeenCalled())
    expect(screen.getByTestId('current-dialog')).toHaveTextContent(
      'group-config'
    )
  })
})
