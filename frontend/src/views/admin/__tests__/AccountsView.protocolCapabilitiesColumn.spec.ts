import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const { listAccounts } = vi.hoisted(() => ({
  listAccounts: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag: vi.fn(),
      getBatchTodayStats: vi.fn().mockResolvedValue({ stats: {} }),
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: vi.fn().mockResolvedValue([]) },
    groups: { getAll: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const ProtocolCapabilityCellStub = defineComponent({
  name: 'OpenAIProtocolCapabilityCell',
  props: { account: { type: Object, default: null } },
  template: '<span data-test="protocol-cell">{{ account?.id }}</span>'
})

const DataTableStub = defineComponent({
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" :data-column="column.key">
        {{ column.sortable ? 'sortable' : 'fixed' }}
      </span>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-protocol_capabilities" :row="row" />
      </div>
    </div>
  `
})

const listRow = {
  id: 42,
  name: 'openai auto',
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  concurrency: 2,
  priority: 1,
  group_ids: [],
  credentials: {},
  extra: {
    openai_protocol_capabilities: {
      responses: { supported: true, probed_at: '2026-05-01T08:00:00Z' }
    }
  }
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        OpenAIProtocolCapabilityCell: ProtocolCapabilityCellStub,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        HelpTooltip: true,
        Icon: true,
        Teleport: true
      }
    }
  })
}

describe('admin AccountsView protocol capabilities column', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({
      items: [listRow],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
  })

  it('adds a visible non-sortable column right after platform/type', async () => {
    const wrapper = mountView()
    await flushPromises()

    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{
      key: string
      label: string
      sortable: boolean
    }>
    const keys = columns.map(column => column.key)
    expect(keys.indexOf('protocol_capabilities')).toBe(keys.indexOf('platform_type') + 1)
    expect(columns.find(column => column.key === 'protocol_capabilities')).toMatchObject({
      label: 'admin.accounts.columns.protocolCapabilities',
      sortable: false
    })
    expect(wrapper.get('[data-column="protocol_capabilities"]').text()).toBe('fixed')
    expect(JSON.parse(localStorage.getItem('account-hidden-columns') || '[]')).not.toContain(
      'protocol_capabilities'
    )
    wrapper.unmount()
  })

  it('wires the account row into the cell', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="protocol-cell"]').text()).toBe('42')
    expect(wrapper.getComponent(ProtocolCapabilityCellStub).props('account')).toMatchObject({
      id: 42,
      platform: 'openai',
      extra: listRow.extra
    })
    wrapper.unmount()
  })

  it('respects an explicit hidden preference for the column', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['protocol_capabilities']))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-column="protocol_capabilities"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
