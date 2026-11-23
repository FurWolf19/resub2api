import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup } from '@/types'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'

const mocks = vi.hoisted(() => ({
  getGroupRateMultipliers: vi.fn(),
  batchSetGroupRateMultipliers: vi.fn(),
  getGroupQuotaMultipliers: vi.fn(),
  batchSetGroupQuotaMultipliers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { groups: mocks } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }),
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

const rateEntry = (userId: number) => ({
  user_id: userId,
  user_email: `user${userId}@example.com`,
  user_name: '',
  user_notes: '',
  user_status: 'active',
  rate_multiplier: 2,
  rpm_override: null,
})

const quotaEntry = (userId: number, overrides: Record<string, unknown> = {}) => ({
  user_id: userId,
  username: '',
  email: `user${userId}@example.com`,
  notes: '',
  status: 'active',
  subscription_id: null,
  quota_multiplier: null,
  default_quota_multiplier: null,
  rate_multiplier: null,
  ...overrides,
})

async function openModal() {
  const wrapper = mount(GroupRateMultipliersModal, {
    props: {
      show: false,
      group: { id: 1, name: 'Group', platform: 'openai', rate_multiplier: 1 } as AdminGroup,
    },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        Icon: true,
        PlatformIcon: true,
        Pagination: true,
        ConfirmDialog: true,
      },
    },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

async function clickSave(wrapper: Awaited<ReturnType<typeof openModal>>) {
  await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getGroupRateMultipliers.mockResolvedValue([])
  mocks.getGroupQuotaMultipliers.mockResolvedValue([])
  mocks.batchSetGroupRateMultipliers.mockResolvedValue(undefined)
  mocks.batchSetGroupQuotaMultipliers.mockResolvedValue(undefined)
})

describe('GroupRateMultipliersModal quota integration', () => {
  it('keeps rate management usable when the quota request fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getGroupRateMultipliers.mockResolvedValue([rateEntry(7)])
    mocks.getGroupQuotaMultipliers.mockRejectedValue(new Error('403'))

    const wrapper = await openModal()

    expect(wrapper.get('tbody').text()).toContain('user7@example.com')
    expect(wrapper.text()).toContain('admin.groups.quotaMultiplierLoadFailed')
    expect(mocks.showError).toHaveBeenCalledWith('admin.groups.quotaMultiplierLoadFailed')

    const quotaInput = wrapper.findAll('tbody input[type="number"]')[1]
    expect(quotaInput.attributes('disabled')).toBeDefined()

    await wrapper.get('tbody input[type="number"]').setValue('3')
    await clickSave(wrapper)

    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [
      { user_id: 7, rate_multiplier: 3 },
    ])
    expect(mocks.batchSetGroupQuotaMultipliers).not.toHaveBeenCalled()
  })

  it('submits a null quota tombstone when a deleted row had quota settings', async () => {
    mocks.getGroupRateMultipliers.mockResolvedValue([rateEntry(7)])
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(7, { quota_multiplier: 0.5 }),
    ])

    const wrapper = await openModal()

    await wrapper.get('tbody tr td:last-child button').trigger('click')
    expect(wrapper.find('tbody tr').exists()).toBe(false)

    await clickSave(wrapper)

    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [])
    expect(mocks.batchSetGroupQuotaMultipliers).toHaveBeenCalledWith(1, [
      { user_id: 7, quota_multiplier: null },
    ])
  })

  it('only labels active quotas and pending defaults', async () => {
    mocks.getGroupRateMultipliers.mockResolvedValue([rateEntry(3)])
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(1, { subscription_id: 11, quota_multiplier: 0.5 }),
      quotaEntry(2, { default_quota_multiplier: 0.3 }),
    ])

    const wrapper = await openModal()
    const rows = wrapper.findAll('tbody tr')
    const rowText = (userId: number) =>
      rows.find(row => row.text().includes(`user${userId}@example.com`))!.text()

    expect(rowText(1)).toContain('admin.groups.quotaMultiplierEffectiveBadge')
    expect(rowText(1)).not.toContain('admin.groups.quotaMultiplierDefaultBadge')

    expect(rowText(2)).toContain('admin.groups.quotaMultiplierDefaultBadge')
    expect(rowText(2)).not.toContain('admin.groups.quotaMultiplierEffectiveBadge')

    expect(rowText(3)).not.toContain('admin.groups.quotaMultiplierDefaultBadge')
    expect(rowText(3)).not.toContain('admin.groups.quotaMultiplierEffectiveBadge')
  })
})

function findQuotaRow(wrapper: Awaited<ReturnType<typeof openModal>>, userId: number) {
  const row = wrapper
    .findAll('tbody tr')
    .find((item) => item.text().includes(`user${userId}@example.com`))
  expect(row, `row for user ${userId}`).toBeTruthy()
  return row!
}

function findQuotaInput(wrapper: Awaited<ReturnType<typeof openModal>>, userId: number) {
  return findQuotaRow(wrapper, userId).findAll('input[type="number"]')[1]
}

function quotaPercentText(wrapper: Awaited<ReturnType<typeof openModal>>, userId: number) {
  return findQuotaRow(wrapper, userId)
    .findAll('span')
    .map((span) => span.text())
    .find((text) => text.endsWith('%'))
}

describe('GroupRateMultipliersModal quota clearing', () => {
  it('clears an active quota to 1.0 instead of falling back to the default', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(1, { subscription_id: 11, quota_multiplier: 0.5, default_quota_multiplier: 0.8 }),
    ])

    const wrapper = await openModal()
    const input = findQuotaInput(wrapper, 1)

    expect((input.element as HTMLInputElement).value).toBe('0.5')
    expect(quotaPercentText(wrapper, 1)).toBe('50.0%')
    await input.setValue('')
    await input.trigger('change')

    expect((input.element as HTMLInputElement).value).toBe('1')
    expect(quotaPercentText(wrapper, 1)).toBe('100.0%')
    expect(findQuotaRow(wrapper, 1).text()).not.toContain('80.0%')
  })

  it('submits 1.0 for a cleared active quota even when a default exists', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(1, { subscription_id: 11, quota_multiplier: 0.5, default_quota_multiplier: 0.8 }),
    ])

    const wrapper = await openModal()
    const input = findQuotaInput(wrapper, 1)
    await input.setValue('')
    await input.trigger('change')
    await clickSave(wrapper)

    expect(mocks.batchSetGroupQuotaMultipliers).toHaveBeenCalledWith(1, [
      { user_id: 1, quota_multiplier: 1 },
    ])
  })

  it('shows 100.0% immediately after clearing an active quota', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(1, { subscription_id: 11, quota_multiplier: 0.5, default_quota_multiplier: 0.8 }),
    ])

    const wrapper = await openModal()
    const input = findQuotaInput(wrapper, 1)
    await input.setValue('')
    await input.trigger('change')

    expect(input.element).toHaveProperty('value', '1')
    expect(quotaPercentText(wrapper, 1)).toBe('100.0%')
  })

  it('binds and clears the default field for users without an active subscription', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(2, { default_quota_multiplier: 0.3 }),
    ])

    const wrapper = await openModal()
    const input = findQuotaInput(wrapper, 2)

    expect((input.element as HTMLInputElement).value).toBe('0.3')
    expect(quotaPercentText(wrapper, 2)).toBe('30.0%')
    await input.setValue('')
    await input.trigger('change')
    expect((input.element as HTMLInputElement).value).toBe('')
    await clickSave(wrapper)

    expect(mocks.batchSetGroupQuotaMultipliers).toHaveBeenCalledWith(1, [
      { user_id: 2, quota_multiplier: null },
    ])
  })

  it('does not render a use-default action', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(1, { subscription_id: 11, quota_multiplier: 0.5 }),
    ])

    const wrapper = await openModal()

    expect(wrapper.text()).not.toContain('quotaMultiplierUseDefault')
    expect(wrapper.findAll('button').map((button) => button.text())).not.toContain(
      'admin.groups.quotaMultiplierUseDefault'
    )
  })
})

describe('GroupRateMultipliersModal header count', () => {
  it('counts only users with a price rate in the header', async () => {
    mocks.getGroupRateMultipliers.mockResolvedValue([rateEntry(3)])
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(4, { default_quota_multiplier: 0.3 }),
    ])

    const wrapper = await openModal()

    expect(wrapper.findAll('h4')[1]!.text()).toContain('admin.groups.rateMultipliers (1)')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
  })

  it('keeps quota-only users visible in the table', async () => {
    mocks.getGroupQuotaMultipliers.mockResolvedValue([
      quotaEntry(4, { default_quota_multiplier: 0.3 }),
      quotaEntry(5, { subscription_id: 12, quota_multiplier: 0.6 }),
    ])

    const wrapper = await openModal()

    expect(wrapper.findAll('h4')[1]!.text()).toContain('admin.groups.rateMultipliers (0)')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('admin.groups.noRateMultipliers')
  })
})
