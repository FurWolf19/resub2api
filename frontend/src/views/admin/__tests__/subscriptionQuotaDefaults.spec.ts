import { describe, expect, it } from 'vitest'

import { collectAssignableQuotaDefaults } from '../subscriptionQuotaDefaults'

function entry(overrides: Record<string, unknown>) {
  return {
    user_id: 1,
    username: '',
    email: 'user@example.com',
    notes: '',
    status: 'active',
    subscription_id: null,
    quota_multiplier: null,
    default_quota_multiplier: null,
    rate_multiplier: null,
    ...overrides,
  }
}

describe('collectAssignableQuotaDefaults', () => {
  it('collects defaults only for users without an active subscription', () => {
    const defaults = collectAssignableQuotaDefaults([
      entry({ user_id: 12, subscription_id: null, default_quota_multiplier: 0.7 }),
      entry({ user_id: 11, subscription_id: 99, default_quota_multiplier: 0.4 }),
    ])

    expect([...defaults.entries()]).toEqual([[12, 0.7]])
  })

  it('ignores null defaults and users with subscription_id', () => {
    const defaults = collectAssignableQuotaDefaults([
      entry({ user_id: 1, subscription_id: null, default_quota_multiplier: null }),
      entry({ user_id: 2, subscription_id: 3, default_quota_multiplier: 0.9 }),
    ])

    expect(defaults.size).toBe(0)
  })

  it('returns an empty map when every entry has an active subscription', () => {
    const defaults = collectAssignableQuotaDefaults([
      entry({ user_id: 1, subscription_id: 10, default_quota_multiplier: 0.5 }),
      entry({ user_id: 2, subscription_id: 11, default_quota_multiplier: 0.6 }),
    ])

    expect(defaults.size).toBe(0)
  })
})
