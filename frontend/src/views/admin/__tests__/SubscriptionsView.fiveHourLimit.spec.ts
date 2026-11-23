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
      <div v-for="row in data" :key="row.id" data-test="usage-row">
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

function subscriptionRow(id: number, fiveHourLimit: number) {
  return {
    id,
    user_id: id,
    group_id: 1,
    status: 'active',
    starts_at: '2026-01-01T00:00:00Z',
    expires_at: null,
    five_hour_usage_usd: 0,
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    five_hour_window_start: null,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    user: { email: `user${id}@example.com` },
    group: {
      id: 1,
      name: 'Group',
      platform: 'openai',
      five_hour_limit_usd: fiveHourLimit,
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null
    }
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
        Pagination: true,
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('admin subscription five-hour limit visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    getAllGroups.mockResolvedValue([])
  })

  it('does not render the five-hour usage row for zero or negative limits', async () => {
    listSubscriptions.mockResolvedValue({
      items: [subscriptionRow(1, 0), subscriptionRow(2, -1)],
      total: 2,
      pages: 1
    })

    const wrapper = await mountView()
    const rows = wrapper.findAll('[data-test="usage-row"]')

    expect(rows).toHaveLength(2)
    for (const row of rows) {
      expect(row.text()).not.toContain('admin.subscriptions.fiveHour')
    }
  })
})
