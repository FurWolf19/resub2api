import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const { listSubscriptions, getAllGroups } = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { list: listSubscriptions },
    groups: { getAll: getAllGroups },
    users: { list: vi.fn().mockResolvedValue({ items: [], total: 0, pages: 0 }) }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id" data-test="quota-row">
        <slot name="cell-quota_multiplier" :row="row" />
      </div>
    </div>
  `
}

const BaseDialogStub = {
  props: ['show', 'title'],
  template: `
    <div v-if="show" data-test="quota-dialog">
      {{ title }}<slot />
    </div>
  `
}

function subscriptionRow(id: number, status: string) {
  return {
    id,
    user_id: id,
    group_id: 1,
    status,
    quota_multiplier: 0.5,
    starts_at: '2026-01-01T00:00:00Z',
    expires_at: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    user: { email: `user${id}@example.com` }
  }
}

async function mountView() {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /></div>'
        },
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        Pagination: true,
        Select: true,
        ConfirmDialog: true,
        Icon: true,
        GroupBadge: true,
        GroupOptionItem: true,
        EmptyState: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('admin subscription quota multiplier entry', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    getAllGroups.mockResolvedValue([])
    listSubscriptions.mockResolvedValue({
      items: [
        subscriptionRow(1, 'active'),
        subscriptionRow(2, 'expired'),
        subscriptionRow(3, 'revoked')
      ],
      total: 3,
      pages: 1
    })
  })

  it('disables the quota edit button for a revoked subscription', async () => {
    const wrapper = await mountView()
    const rows = wrapper.findAll('[data-test="quota-row"]')
    const revokedButton = rows[2]!.get('button')

    expect(revokedButton.attributes('disabled')).toBeDefined()
    expect(revokedButton.attributes('title')).toBe(
      'admin.subscriptions.quotaMultiplierRevokedHint'
    )
  })

  it('keeps the quota edit button enabled for active and expired subscriptions', async () => {
    const wrapper = await mountView()
    const rows = wrapper.findAll('[data-test="quota-row"]')

    for (const row of [rows[0]!, rows[1]!]) {
      const button = row.get('button')
      expect(button.attributes('disabled')).toBeUndefined()
      expect(button.attributes('title')).toBe('admin.subscriptions.editQuotaMultiplier')
      expect(button.text()).toBe('50.0%')
    }
  })

  it('does not open the quota dialog for a revoked subscription', async () => {
    const wrapper = await mountView()
    const revokedButton = wrapper.findAll('[data-test="quota-row"]')[2]!.get('button')

    await revokedButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="quota-dialog"]').exists()).toBe(false)
  })

  it('opens the quota dialog for an active subscription', async () => {
    const wrapper = await mountView()
    const activeButton = wrapper.findAll('[data-test="quota-row"]')[0]!.get('button')

    await activeButton.trigger('click')
    await flushPromises()

    const dialog = wrapper.get('[data-test="quota-dialog"]')
    expect(dialog.text()).toContain('admin.subscriptions.editQuotaMultiplierTitle')
    expect(dialog.text()).toContain('50.0%')
  })
})
