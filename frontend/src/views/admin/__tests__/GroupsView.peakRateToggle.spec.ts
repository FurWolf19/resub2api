import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'

import GroupsView from '../GroupsView.vue'

const {
  listGroups,
  getAllGroups,
  getModelAllowlistCandidates,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability,
  listAccounts,
  createGroup,
  updateGroup,
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
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.groups.createGroup': 'Create Group',
  'admin.groups.peakRate.enable': 'Enable peak rate multiplier',
  'admin.groups.peakRate.emptyError': 'Add at least one peak window',
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
      create: createGroup,
      update: updateGroup,
      delete: vi.fn(),
      updateSortOrder: vi.fn(),
    },
    accounts: {
      list: listAccounts,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
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

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
}

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: '<div><slot name="empty" /></div>',
}

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

const SelectStub = {
  props: ['modelValue', 'options', 'placeholder'],
  emits: ['update:modelValue', 'change'],
  template: '<select />',
}

const IconStub = {
  props: ['name'],
  template: '<span>{{ name }}</span>',
}

const ReasoningEffortPolicyFieldsStub = defineComponent({
  name: 'ReasoningEffortPolicyFields',
  setup(_props, { expose }) {
    expose({ validate: () => true, resetValidation: () => {} })
    return {}
  },
  template: '<div />',
})

const PeakRateWindowsEditorStub = defineComponent({
  name: 'PeakRateWindowsEditor',
  props: ['modelValue'],
  setup(_props, { expose }) {
    expose({ validate: () => null })
    return {}
  },
  template: '<div data-test="peak-rate-editor" />',
})

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
        GroupI18nField: true,
        ReasoningEffortPolicyFields: ReasoningEffortPolicyFieldsStub,
        PeakRateWindowsEditor: PeakRateWindowsEditorStub,
        VueDraggable: { template: '<div><slot /></div>' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

async function openCreateModal(wrapper: Awaited<ReturnType<typeof mountView>>) {
  await wrapper
    .findAll('button')
    .find((button) => button.text().includes('Create Group'))!
    .trigger('click')
  await flushPromises()
}

function peakRateCheckbox(wrapper: Awaited<ReturnType<typeof mountView>>) {
  const label = wrapper
    .findAll('label')
    .find((item) => item.text().includes('Enable peak rate multiplier'))
  expect(label, 'peak rate toggle').toBeTruthy()
  return label!.get('input[type="checkbox"]')
}

function groupFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    name: 'Edit Peak Group',
    description: '',
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
    long_context_pricing_enabled: true,
    force_openai_fast: false,
    free_openai_fast: false,
    model_pricing: [],
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null,
    video_price_720p: null,
    video_price_1080p: null,
    peak_rate_enabled: true,
    peak_start: '09:00',
    peak_end: '12:00',
    peak_rate_multiplier: 2,
    peak_rate_windows: [{ start: '09:00', end: '12:00', multiplier: 2, days: [] }],
    profit_control_enabled: false,
    profit_min_margin: 0,
    profit_safety_buffer: 0,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    allow_messages_dispatch: false,
    allow_live: false,
    require_oauth_only: false,
    require_privacy_set: false,
    model_routing: null,
    model_routing_enabled: false,
    supported_model_scopes: [],
    mcp_xml_inject: true,
    copy_accounts_from_group_ids: [],
    max_reasoning_effort: '',
    max_reasoning_effort_over_limit: 'downgrade',
    reasoning_effort_mappings: [],
    model_allowlist: undefined,
    codex_models_manifest_config: undefined,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    sort_order: 0,
    ...overrides,
  }
}

describe('admin GroupsView peak rate toggle wiring', () => {
  beforeEach(() => {
    localStorage.clear()
    listGroups.mockReset().mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0,
    })
    getAllGroups.mockReset().mockResolvedValue([])
    getModelAllowlistCandidates.mockReset().mockResolvedValue([])
    getUsageSummary.mockReset().mockResolvedValue([])
    getCapacitySummary.mockReset().mockResolvedValue([])
    getLiveCapability.mockReset().mockResolvedValue({ supported: false })
    listAccounts.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    createGroup.mockReset().mockResolvedValue({})
    updateGroup.mockReset().mockResolvedValue({})
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('seeds, prunes and submits peak windows according to the toggle', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      createForm: {
        name: string
        subscription_type: string
        peak_rate_windows: Array<Record<string, unknown>>
      }
    }

    vm.createForm.name = 'Peak Group'
    vm.createForm.subscription_type = 'subscription'
    await nextTick()
    await openCreateModal(wrapper)

    let checkbox = peakRateCheckbox(wrapper)
    await checkbox.setValue(true)
    await nextTick()

    expect(vm.createForm.peak_rate_windows).toHaveLength(1)
    expect(vm.createForm.peak_rate_windows[0]).toMatchObject({ start: '', end: '' })

    const complete = {
      ...vm.createForm.peak_rate_windows[0]!,
      start: '09:00',
      end: '12:00',
      multiplier: 2,
    }
    const blank = {
      ...vm.createForm.peak_rate_windows[0]!,
      id: 'blank-window',
      start: '',
      end: '',
    }
    vm.createForm.peak_rate_windows = [complete, blank]
    await nextTick()

    await checkbox.setValue(false)
    await nextTick()

    expect(vm.createForm.peak_rate_windows).toHaveLength(1)
    expect(vm.createForm.peak_rate_windows[0]).toMatchObject({
      start: '09:00',
      end: '12:00',
    })

    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenLastCalledWith(
      expect.objectContaining({ peak_rate_enabled: false, peak_rate_windows: [] })
    )

    vm.createForm.name = 'Peak Group Enabled'
    vm.createForm.subscription_type = 'subscription'
    await nextTick()
    await openCreateModal(wrapper)

    checkbox = peakRateCheckbox(wrapper)
    await checkbox.setValue(true)
    await nextTick()

    const seeded = vm.createForm.peak_rate_windows[0]!
    vm.createForm.peak_rate_windows = [
      { ...seeded, start: '09:00', end: '12:00', multiplier: 2 },
      { ...seeded, id: 'blank-window-enabled', start: '', end: '' },
    ]
    await nextTick()

    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenLastCalledWith(
      expect.objectContaining({
        peak_rate_enabled: true,
        peak_rate_windows: [
          { start: '09:00', end: '12:00', multiplier: 2, days: [] },
        ],
      })
    )
  })

  it('submits an empty peak window array when the edit toggle is turned off', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleEdit: (group: Record<string, unknown>) => Promise<void>
    }

    await vm.handleEdit(groupFixture())
    await flushPromises()

    const checkbox = peakRateCheckbox(wrapper)
    expect((checkbox.element as HTMLInputElement).checked).toBe(true)
    await checkbox.setValue(false)
    await nextTick()

    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({ peak_rate_enabled: false, peak_rate_windows: [] })
    )
  })

  it('submits only complete peak windows when editing with the toggle on', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleEdit: (group: Record<string, unknown>) => Promise<void>
      editForm: { peak_rate_windows: Array<Record<string, unknown>> }
    }

    await vm.handleEdit(groupFixture())
    await flushPromises()

    const seeded = vm.editForm.peak_rate_windows[0]!
    vm.editForm.peak_rate_windows = [
      { ...seeded, start: '09:00', end: '12:00', multiplier: 2 },
      { ...seeded, id: 'blank-edit-window', start: '', end: '' },
    ]
    await nextTick()

    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({
        peak_rate_enabled: true,
        peak_rate_windows: [
          { start: '09:00', end: '12:00', multiplier: 2, days: [] },
        ],
      })
    )
  })

  it('seeds and prunes edit windows when the edit toggle is actually clicked', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as {
      handleEdit: (group: Record<string, unknown>) => Promise<void>
      editForm: { peak_rate_windows: Array<Record<string, unknown>> }
    }

    await vm.handleEdit(
      groupFixture({
        peak_rate_enabled: false,
        peak_start: '',
        peak_end: '',
        peak_rate_multiplier: 1,
        peak_rate_windows: [],
      })
    )
    await flushPromises()

    const checkbox = peakRateCheckbox(wrapper)
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)

    await checkbox.setValue(true)
    await nextTick()

    expect(vm.editForm.peak_rate_windows).toHaveLength(1)
    expect(vm.editForm.peak_rate_windows[0]).toMatchObject({ start: '', end: '' })

    const seeded = vm.editForm.peak_rate_windows[0]!
    vm.editForm.peak_rate_windows = [
      { ...seeded, start: '09:00', end: '12:00', multiplier: 2 },
      { ...seeded, id: 'blank-edit-window-toggle', start: '', end: '' },
    ]
    await nextTick()

    await checkbox.setValue(false)
    await nextTick()

    expect(vm.editForm.peak_rate_windows).toHaveLength(1)
    expect(vm.editForm.peak_rate_windows[0]).toMatchObject({
      start: '09:00',
      end: '12:00',
    })

    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenLastCalledWith(
      7,
      expect.objectContaining({ peak_rate_enabled: false, peak_rate_windows: [] })
    )
  })
})
