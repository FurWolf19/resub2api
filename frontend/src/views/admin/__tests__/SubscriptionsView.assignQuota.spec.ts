import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const {
  listSubscriptions,
  getAllGroups,
  listUsers,
  assignSubscription,
  bulkAssign,
  getGroupQuotaMultipliers,
  showError
} = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getAllGroups: vi.fn(),
  listUsers: vi.fn(),
  assignSubscription: vi.fn(),
  bulkAssign: vi.fn(),
  getGroupQuotaMultipliers: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      assign: assignSubscription,
      bulkAssign
    },
    groups: {
      getAll: getAllGroups,
      getGroupQuotaMultipliers
    },
    users: { list: listUsers }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${Object.values(params).join(',')}` : key
    })
  }
})

const DataTableStub = {
  name: 'DataTable',
  props: ['data', 'selectedKeys'],
  emits: ['update:selectedKeys', 'sort'],
  template: '<div />'
}

function mountView() {
  return mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Pagination: true,
        Select: true,
        ConfirmDialog: true,
        Icon: true,
        GroupBadge: true,
        GroupOptionItem: true,
        EmptyState: true,
        Teleport: true,
        RouterLink: true
      }
    }
  })
}

async function openAssignModal(wrapper: ReturnType<typeof mountView>) {
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'admin.subscriptions.assignSubscription')!
    .trigger('click')
  await flushPromises()
  return wrapper.get('#assign-subscription-form')
}

async function selectGroup(form: DOMWrapper<Element>) {
  form.getComponent({ name: 'Select' }).vm.$emit('update:modelValue', 7)
  await flushPromises()
}

async function addUser(form: DOMWrapper<Element>, id: number) {
  listUsers.mockResolvedValue({
    items: [{ id, email: `user${id}@example.com` }],
    total: 1,
    pages: 1
  })
  const search = form.get('[data-assign-user-search] input')
  await search.trigger('focus')
  await search.setValue(`user${id}`)
  await vi.advanceTimersByTimeAsync(300)
  await flushPromises()
  await form.get('[data-assign-user-search] button').trigger('click')
  await flushPromises()
}

async function enableAssignQuota(form: DOMWrapper<Element>) {
  const toggle = form
    .findAll('label')
    .find((label) => label.text().includes('admin.subscriptions.assignQuotaMultiplier'))!
    .get('input[type="checkbox"]')
  await toggle.setValue(true)
  await flushPromises()
  return form
    .findAll('input[type="number"]')
    .find((input) => input.attributes('max') === '1')!
}

describe('admin subscription assignment quota defaults', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 777 }))
    listSubscriptions.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getAllGroups.mockResolvedValue([
      {
        id: 7,
        name: 'Subscription Group',
        description: null,
        platform: 'openai',
        subscription_type: 'subscription',
        status: 'active',
        rate_multiplier: 1
      }
    ])
    getGroupQuotaMultipliers.mockResolvedValue([])
    assignSubscription.mockResolvedValue({})
    bulkAssign.mockResolvedValue({
      success_count: 1,
      created_count: 1,
      reused_count: 0,
      failed_count: 1,
      subscriptions: [{ user_id: 11 }],
      errors: []
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not prefill an assignment default for a user with an active subscription', async () => {
    getGroupQuotaMultipliers.mockResolvedValue([
      { user_id: 11, subscription_id: 99, default_quota_multiplier: 0.4 }
    ])
    const wrapper = mountView()
    await flushPromises()

    const form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)

    expect(getGroupQuotaMultipliers).toHaveBeenCalledWith(7)
    expect(form.text()).not.toContain('admin.subscriptions.defaultQuotaMultiplierHint')
  })

  it('does not drop the group default cache when the modal closes without assigning', async () => {
    getGroupQuotaMultipliers.mockResolvedValue([
      { user_id: 11, subscription_id: null, default_quota_multiplier: 0.4 }
    ])
    const wrapper = mountView()
    await flushPromises()

    let form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)
    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(1)
    expect(form.text()).toContain('admin.subscriptions.defaultQuotaMultiplierHint')
    expect(form.text()).toContain('40.0%')

    await wrapper.findAll('button').find((button) => button.text() === 'common.cancel')!.trigger('click')
    await flushPromises()

    form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)

    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(1)
    expect(form.text()).toContain('admin.subscriptions.defaultQuotaMultiplierHint')
    expect(form.text()).toContain('40.0%')
    const quotaInput = await enableAssignQuota(form)
    expect((quotaInput.element as HTMLInputElement).value).toBe('0.4')
  })

  it('invalidates the group default cache after a successful single assignment', async () => {
    const wrapper = mountView()
    await flushPromises()

    let form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)
    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(1)

    await form.trigger('submit')
    await flushPromises()

    form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)

    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(2)
  })

  it('discards a stale defaults response that resolves after assignment invalidation', async () => {
    let resolveFirstRequest!: (value: unknown) => void
    getGroupQuotaMultipliers.mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveFirstRequest = resolve
      })
    )

    const wrapper = mountView()
    await flushPromises()

    let form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)
    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(1)

    await form.trigger('submit')
    await flushPromises()

    resolveFirstRequest([
      { user_id: 11, subscription_id: 99, default_quota_multiplier: 0.4 }
    ])
    await flushPromises()

    form = await openAssignModal(wrapper)
    await selectGroup(form)
    await addUser(form, 11)

    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(2)
    expect(form.text()).not.toContain('admin.subscriptions.defaultQuotaMultiplierHint')
    expect(form.text()).not.toContain('40.0%')
    const quotaInput = await enableAssignQuota(form)
    expect((quotaInput.element as HTMLInputElement).value).toBe('1')
  })

  it('invalidates the group default cache after a successful batch assignment', async () => {
    const wrapper = mountView()
    await flushPromises()

    const form = await openAssignModal(wrapper)
    await form.get('input[type="checkbox"]').setValue(true)
    await selectGroup(form)
    await addUser(form, 11)
    await addUser(form, 22)
    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(1)

    await form.trigger('submit')
    await flushPromises()

    expect(bulkAssign).toHaveBeenCalledWith({
      user_ids: [11, 22],
      group_id: 7,
      validity_days: 30
    })
    expect(getGroupQuotaMultipliers).toHaveBeenCalledTimes(2)
  })
})
