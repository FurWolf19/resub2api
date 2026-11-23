import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionProgressMini from '../SubscriptionProgressMini.vue'
const store = vi.hoisted(() => ({ activeSubscriptions: [] as unknown[], hasActiveSubscriptions: true, fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined) }))
vi.mock('@/stores', () => ({ useSubscriptionStore: () => store }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: { subscription: 'subscription' }, isFeatureFlagEnabled: () => true }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 22, 12)) })
afterEach(() => vi.useRealTimers())
describe('subscription expiry calendar labels', () => {
  it.each([
    [new Date(2026, 8, 22, 18), 'expiresToday'],
    [new Date(2026, 8, 23, 18), 'expiresTomorrow'],
    [new Date(2026, 8, 22, 12), 'expired'],
    [new Date(2026, 8, 25, 12), 'daysRemaining'],
  ])('labels %s as %s', async (expires, label) => {
    store.activeSubscriptions = [{ id: 1, group_id: 1, expires_at: expires.toISOString(), group: { name: 'Plan' } }]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')
    expect(w.text()).toContain('subscriptionProgress.' + label)
  })
})

describe('subscription progress quota limit visibility', () => {
  const subscriptionWithLimits = (limits: Record<string, number | null>) => ({
    id: 2,
    group_id: 1,
    status: 'active',
    quota_multiplier: 1,
    expires_at: null,
    five_hour_usage_usd: 0,
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    group: { name: 'Plan', ...limits },
  })

  it('treats zero and negative limits as unset and shows unlimited', async () => {
    store.activeSubscriptions = [
      subscriptionWithLimits({
        five_hour_limit_usd: 0,
        daily_limit_usd: -1,
        weekly_limit_usd: 0,
        monthly_limit_usd: -1,
      }),
    ]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')

    expect(w.text()).toContain('subscriptionProgress.unlimited')
    expect(w.text()).not.toContain('subscriptionProgress.fiveHour')
    expect(w.text()).not.toContain('$-1.00')
    expect(w.text()).not.toContain('$0.00/$')
  })

  it('hides a zero five-hour limit while keeping positive daily limits', async () => {
    store.activeSubscriptions = [
      subscriptionWithLimits({
        five_hour_limit_usd: 0,
        daily_limit_usd: 10,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
      }),
    ]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')

    expect(w.text()).toContain('subscriptionProgress.daily')
    expect(w.text()).toContain('$0.00/$10.00')
    expect(w.text()).not.toContain('subscriptionProgress.fiveHour')
  })

  it('hides a negative five-hour limit instead of rendering a negative amount', async () => {
    store.activeSubscriptions = [
      subscriptionWithLimits({
        five_hour_limit_usd: -1,
        daily_limit_usd: 10,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
      }),
    ]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')

    expect(w.text()).not.toContain('subscriptionProgress.fiveHour')
    expect(w.text()).not.toContain('$-1.00')
  })
})
