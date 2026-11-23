import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions } = vi.hoisted(() => ({
  getMySubscriptions: vi.fn()
}))

vi.mock('@/api/subscriptions', () => ({
  default: { getMySubscriptions }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), cachedPublicSettings: undefined })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
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

function subscription(id: number, fiveHourLimit: number) {
  return {
    id,
    user_id: 1,
    group_id: 1,
    status: 'active',
    starts_at: '2026-01-01T00:00:00Z',
    expires_at: null,
    quota_multiplier: 1,
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
    group: {
      id: 1,
      name: 'Group',
      platform: 'openai',
      rate_multiplier: 1,
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
        Icon: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('user subscription five-hour limit visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not render the five-hour progress block for zero or negative limits', async () => {
    getMySubscriptions.mockResolvedValue([subscription(1, 0), subscription(2, -1)])

    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('userSubscriptions.unlimited')
    expect(text).not.toContain('userSubscriptions.fiveHour')
  })

  it('hides quota multiplier badges and keeps effective limits scaled', async () => {
    const sub = subscription(1, 0.02)
    sub.quota_multiplier = 0.5
    sub.group.daily_limit_usd = 0.06
    getMySubscriptions.mockResolvedValue([sub])

    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).not.toContain('40.0%')
    expect(text).not.toContain('50.0%')
    expect(text).not.toContain('额度倍率')
    expect(text).not.toContain('Quota multiplier')
    expect(text).not.toContain('quotaMultiplierNote')
    expect(text).toContain('$0.00 / $0.01')
    expect(text).toContain('$0.00 / $0.03')
  })
})
