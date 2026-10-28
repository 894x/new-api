import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type {
  User,
  UserChannelRoutingConfig,
  UserGroupConfig,
} from '../../types'
import { DataTableRowActions } from '../data-table-row-actions'
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

function setGroupPermissions(read: boolean, write: boolean, role = 10) {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'operator',
    role,
    permissions: { admin_permissions: { user_group_config: { read, write } } },
  })
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
  it.each([
    { read: true, write: false, role: 10, targetRole: 1, visible: true },
    { read: true, write: true, role: 10, targetRole: 10, visible: false },
    { read: false, write: true, role: 10, targetRole: 1, visible: false },
    { read: true, write: true, role: 1, targetRole: 1, visible: false },
    { read: false, write: false, role: 100, targetRole: 100, visible: true },
  ])(
    'shows the row entry only to authorized managers (%j)',
    async (permission) => {
      setGroupPermissions(permission.read, permission.write, permission.role)
      const client = new QueryClient()
      render(
        <QueryClientProvider client={client}>
          <UsersProvider>
            <CurrentDialog />
            <DataTableRowActions
              row={
                {
                  original: { ...user, role: permission.targetRole },
                } as Row<User>
              }
            />
          </UsersProvider>
        </QueryClientProvider>
      )
      const menu = screen.queryByRole('button', { name: 'Open menu' })
      if (menu) await userEvent.click(menu)
      const entry = screen.queryByRole('menuitem', {
        name: 'Customer group configuration',
      })
      if (!permission.visible) {
        expect(entry).not.toBeInTheDocument()
        return
      }
      expect(entry).toBeInTheDocument()
      await userEvent.click(
        screen.getByRole('menuitem', { name: 'Customer group configuration' })
      )
      expect(screen.getByTestId('current-dialog')).toHaveTextContent(
        'group-config'
      )
    }
  )

  it.each([
    { read: false, write: false, role: 10 },
    { read: false, write: true, role: 10 },
    { read: true, write: true, role: 1 },
  ])(
    'does not fetch or open a directly mounted drawer without read access (%j)',
    (permission) => {
      setGroupPermissions(permission.read, permission.write, permission.role)
      renderGroupConfig()
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      expect(api.get).not.toHaveBeenCalled()
    }
  )

  it('keeps every tab and preview usable while disabling mutations for a read-only manager', async () => {
    setGroupPermissions(true, false)
    vi.mocked(api.post).mockResolvedValue({
      data: { success: true, data: { 'model-x': { candidate_count: 1 } } },
    })
    renderGroupConfig()
    expect(
      await screen.findByRole('button', { name: 'Create dedicated group' })
    ).toBeDisabled()
    expect(screen.getByRole('spinbutton', { name: 'Ratio' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Add ratio override' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Add model ratio' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    fireEvent.click(screen.getByRole('tab', { name: 'Rate limits' }))
    expect(
      screen.getByRole('switch', { name: 'Enable group rate limit' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('spinbutton', { name: 'Max requests per period' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Add model rule' })
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('tab', { name: 'Channels' }))
    expect(
      await screen.findByRole('spinbutton', {
        name: 'Priority for Pool Alpha #11',
      })
    ).toBeDisabled()
    expect(
      screen.getByRole('switch', {
        name: 'Enable Pool Alpha #11 for this user',
      })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('textbox', { name: 'Model' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Add rule' })).toBeDisabled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Preview channel availability' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/user/42/group-config/preview',
        expect.any(Object)
      )
    )
    fireEvent.submit(
      screen.getByRole('form', { name: 'Customer group configuration' })
    )
    await waitFor(() => expect(api.put).not.toHaveBeenCalled())
    expect(api.patch).not.toHaveBeenCalled()
  })

  it('closes an open ratio editor and blocks saving when write permission is revoked', async () => {
    setGroupPermissions(true, true)
    renderGroupConfig()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Add model ratio' })
    )
    expect(
      screen.getByRole('dialog', { name: 'Add model ratio' })
    ).toBeInTheDocument()
    act(() => setGroupPermissions(true, false))
    expect(
      screen.queryByRole('dialog', { name: 'Add model ratio' })
    ).not.toBeInTheDocument()
    fireEvent.submit(
      screen.getByRole('form', { name: 'Customer group configuration' })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save changes' })
      ).toBeDisabled()
    )
    expect(api.put).not.toHaveBeenCalled()
  })

  it('prevents an already-open dedicated-group confirmation after write permission is revoked', async () => {
    setGroupPermissions(true, true)
    renderGroupConfig()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Create dedicated group' })
    )
    const confirmation = screen.getByRole('alertdialog')
    act(() => setGroupPermissions(true, false))
    expect(
      within(confirmation).getByRole('button', {
        name: 'Create dedicated group',
      })
    ).toBeDisabled()
    fireEvent.click(
      within(confirmation).getByRole('button', {
        name: 'Create dedicated group',
      })
    )
    expect(api.post).not.toHaveBeenCalled()
  })

  it('lets a delegated manager save group discounts and open group policies from the user editor', async () => {
    setGroupPermissions(true, true)
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
    const view = renderGroupConfig()
    const ratio = await screen.findByRole('spinbutton', { name: 'Ratio' })
    fireEvent.change(ratio, { target: { value: '0.6' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith(
        '/api/user/42/group-config',
        expect.objectContaining({ discounts: { default: 0.6 } })
      )
    )
    view.unmount()
    renderUserEditor()
    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/42'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure group policies' })
    )
    expect(screen.getByTestId('current-dialog')).toHaveTextContent(
      'group-config'
    )
  })

  it('reopens with the saved group configuration while the existing query is still fresh', async () => {
    const saved = {
      ...sharedConfig,
      revision: 'revision-2',
      discounts: { default: 0.66 },
      model_discounts: { default: { 'public-custom': 0.42 } },
    }
    vi.mocked(api.put).mockResolvedValue({
      data: { success: true, data: saved },
    })
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity },
        mutations: { retry: false },
      },
    })
    const onOpenChange = vi.fn()
    const view = render(
      <UserGroupConfigDrawer open onOpenChange={onOpenChange} user={user} />,
      {
        wrapper: ({ children }) => (
          <QueryClientProvider client={client}>
            <UsersProvider>{children}</UsersProvider>
          </QueryClientProvider>
        ),
      }
    )
    fireEvent.change(await screen.findByRole('spinbutton', { name: 'Ratio' }), {
      target: { value: '0.66' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    view.unmount()
    render(
      <QueryClientProvider client={client}>
        <UsersProvider>
          <UserGroupConfigDrawer open onOpenChange={onOpenChange} user={user} />
        </UsersProvider>
      </QueryClientProvider>
    )
    expect(
      await screen.findByRole('spinbutton', { name: 'Ratio' })
    ).toHaveValue(0.66)
    expect(screen.getByText('public-custom')).toBeInTheDocument()
    expect(screen.getByText('0.42')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledTimes(1)
  })

  it('saves public model ratios under each key group and keeps edits local until the drawer is saved', async () => {
    const operator = userEvent.setup()
    const configured = {
      ...sharedConfig,
      model_discounts: {
        default: { 'public-a': 0.2 },
        vip: { 'public-a': 0.8 },
      },
    }
    vi.mocked(api.get).mockImplementation(async (url) => ({
      data: {
        success: true,
        data: url === '/api/channel/models_enabled' ? [] : configured,
      },
    }))
    vi.mocked(api.put).mockResolvedValue({
      data: { success: true, data: configured },
    })
    renderGroupConfig()
    await screen.findByText('Model special ratio rules')
    await operator.click(
      screen.getAllByRole('button', { name: 'Edit model ratio' })[0]
    )
    const dialog = screen.getByRole('dialog', { name: 'Edit model ratio' })
    await waitFor(() =>
      expect(api.get).toHaveBeenCalledWith('/api/channel/models_enabled')
    )
    const ratio = within(dialog).getByRole('spinbutton', { name: 'Ratio' })
    await operator.clear(ratio)
    await operator.type(ratio, '0')
    await operator.click(within(dialog).getByRole('button', { name: 'Apply' }))
    expect(api.put).not.toHaveBeenCalled()
    await operator.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith(
        '/api/user/42/group-config',
        expect.objectContaining({
          group: 'default',
          revision: 'revision-1',
          model_discounts: {
            default: { 'public-a': 0 },
            vip: { 'public-a': 0.8 },
          },
        })
      )
    )
  })

  it('lets a delegated group manager apply a custom model ratio without channel-list permission', async () => {
    const operator = userEvent.setup()
    setGroupPermissions(true, true)
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'operator',
      role: 10,
      permissions: {
        admin_permissions: {
          user_group_config: { read: true, write: true },
          channel: { read: false },
        },
      },
    })
    renderGroupConfig()
    await operator.click(
      await screen.findByRole('button', { name: 'Add model ratio' })
    )
    const dialog = screen.getByRole('dialog', { name: 'Add model ratio' })
    const model = within(dialog).getByRole('combobox', {
      name: 'Public model ID',
    })
    await operator.type(model, 'public-custom')
    await operator.keyboard('{Enter}')
    await operator.click(within(dialog).getByRole('button', { name: 'Apply' }))
    expect(await screen.findByText('public-custom')).toBeInTheDocument()
    expect(
      screen.queryByRole('dialog', { name: 'Add model ratio' })
    ).not.toBeInTheDocument()
    expect(api.get).not.toHaveBeenCalledWith('/api/channel/models_enabled')
    expect(api.put).not.toHaveBeenCalled()
  })
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url === '/api/channel/models_enabled') {
        return { data: { success: true, data: [] } }
      }
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

  it('shows an independent outlet group editor as soon as a model has selected pools', async () => {
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: { 'model-x': { candidate_count: 1 } },
      },
    })
    renderGroupConfig()

    fireEvent.click(await screen.findByRole('tab', { name: 'Channels' }))
    expect(await screen.findByText('Pool Alpha #11')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'pool-a' })).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith(
      '/api/user/42/channel-routing-overrides',
      {
        params: {
          model: 'model-x',
          routing_group: 'pool-a',
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
