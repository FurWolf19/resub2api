import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { AdminGroup } from '@/types'
import GroupsView from '../GroupsView.vue'

const {
  listGroups,
  getAllGroups,
  getModelAllowlistCandidates,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability,
  listAccounts,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAllGroups: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  listAccounts: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.groups.columnSettings': 'Column Settings',
  'admin.groups.columns.name': 'Name',
  'admin.groups.columns.id': 'ID',
  'admin.groups.columns.platform': 'Platform',
  'admin.groups.columns.billingType': 'Billing Type',
  'admin.groups.columns.rateMultiplier': 'Rate Multiplier',
  'admin.groups.columns.type': 'Type',
  'admin.groups.columns.accounts': 'Accounts',
  'admin.groups.columns.capacity': 'Capacity',
  'admin.groups.columns.usage': 'Usage',
  'admin.groups.columns.status': 'Status',
  'admin.groups.columns.actions': 'Actions',
  'admin.groups.usageToday': 'Today',
  'admin.groups.usageYesterday': 'Yesterday',
  'admin.groups.usageTotal': 'Total',
  'admin.groups.limit5h': '5h',
  'admin.groups.limitDay': 'd',
  'admin.groups.limitWeek': 'w',
  'admin.groups.limitMonth': 'mo',
  'admin.groups.subscription.subscription': 'Subscription',
  'admin.groups.subscription.standard': 'Standard',
  'admin.groups.subscription.noLimit': 'No limit',
  'admin.groups.subscription.baseLimitHint': 'Base limits, scaled by quota multiplier',
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: getAllGroups,
      getModelAllowlistCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn(),
    },
    accounts: {
      list: listAccounts,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: false }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn().mockReturnValue(false),
    nextStep: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const createGroup = (overrides: Partial<AdminGroup> = {}): AdminGroup => ({
  id: 1,
  name: 'Core Anthropic',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'subscription',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  five_hour_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  default_mapped_model: '',
  messages_dispatch_model_config: undefined,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 3,
  active_account_count: 2,
  rate_limited_account_count: 1,
  model_allowlist: undefined,
  sort_order: 10,
  ...overrides,
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: `
    <div>
      <slot name="filters" />
      <slot name="table" />
    </div>
  `,
}

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div v-if="data.length" data-test="billing-cell">
        <slot name="cell-billing_type" :row="data[0]" />
      </div>
      <div v-if="data.length" data-test="usage-cell">
        <slot name="cell-usage" :row="data[0]" />
      </div>
    </div>
  `,
}

const SelectStub = {
  props: ['modelValue', 'options', 'placeholder'],
  emits: ['update:modelValue', 'change'],
  template: '<select />',
}

const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async () => {
  const wrapper = mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        PlatformIcon: true,
        Icon: IconStub,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: { template: '<div><slot /></div>' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('admin GroupsView billing type cell', () => {
  beforeEach(() => {
    localStorage.clear()

    listGroups.mockReset()
    getAllGroups.mockReset()
    getModelAllowlistCandidates.mockReset()
    getUsageSummary.mockReset()
    getCapacitySummary.mockReset()
    getLiveCapability.mockReset()
    listAccounts.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listGroups.mockResolvedValue({
      items: [createGroup()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getAllGroups.mockResolvedValue([])
    getModelAllowlistCandidates.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getLiveCapability.mockResolvedValue({ supported: false })
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('renders limits without duplicating today usage in the billing type cell', async () => {
    listGroups.mockResolvedValue({
      items: [
        createGroup({
          five_hour_limit_usd: 2,
          daily_limit_usd: 10,
        }),
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getUsageSummary.mockResolvedValue([
      { group_id: 1, today_cost: 3.5, yesterday_cost: 0, total_cost: 9 },
    ])

    const wrapper = await mountView()
    const text = wrapper.get('[data-test="billing-cell"]').text()

    expect(text).toContain('$2.00')
    expect(text).toContain('$10.00')
    expect(text).toContain('5h')
    expect(text).toContain('d')
    expect(text).not.toContain('$3.50')
    expect(text).not.toContain('Today')
    expect(text).not.toContain('Total')
    expect(text).not.toContain('3.50 /')
  })

  it('does not render $0.00 inside the billing type cell for zero usage', async () => {
    listGroups.mockResolvedValue({
      items: [createGroup({ daily_limit_usd: 10 })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getUsageSummary.mockResolvedValue([
      { group_id: 1, today_cost: 0, yesterday_cost: 0, total_cost: 0 },
    ])

    const wrapper = await mountView()

    expect(wrapper.get('[data-test="billing-cell"]').text()).not.toContain('$0.00 /')
    expect(wrapper.get('[data-test="usage-cell"]').text()).toContain('Today$0.00')
  })

  it('renders the base-limit hint without computing discounted values', async () => {
    listGroups.mockResolvedValue({
      items: [createGroup({ daily_limit_usd: 10 })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = await mountView()
    const text = wrapper.get('[data-test="billing-cell"]').text()

    expect(text).toContain('Base limits, scaled by quota multiplier')
    expect(text).not.toContain('$20.00')
  })

  it('shows noLimit when no subscription limit is configured', async () => {
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="billing-cell"]').text()

    expect(text).toContain('No limit')
    expect(text).not.toContain('$')
  })

  it('does not fetch usage summary when only the billing type column is visible', async () => {
    localStorage.setItem('group-hidden-columns', JSON.stringify(['usage', 'capacity']))
    localStorage.setItem('group-column-settings-version', '2')

    const wrapper = await mountView()

    expect(wrapper.get('[data-test="billing-cell"]').exists()).toBe(true)
    expect(getUsageSummary).not.toHaveBeenCalled()
  })

  it('fetches usage summary when the usage column is visible', async () => {
    await mountView()

    expect(getUsageSummary).toHaveBeenCalledTimes(1)
  })

  it('shows the base-limit hint in the create subscription limit block', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      createForm: { subscription_type: string }
      openCreateModal: () => void
    }

    vm.createForm.subscription_type = 'subscription'
    await nextTick()
    vm.openCreateModal()
    await flushPromises()

    expect(wrapper.get('#create-group-form').text()).toContain(
      'Base limits, scaled by quota multiplier'
    )
  })

  it('shows the base-limit hint in the edit subscription limit block', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleEdit: (group: AdminGroup) => Promise<void>
    }

    await vm.handleEdit(createGroup({ subscription_type: 'subscription' }))
    await flushPromises()

    expect(wrapper.get('#edit-group-form').text()).toContain(
      'Base limits, scaled by quota multiplier'
    )
  })
})
