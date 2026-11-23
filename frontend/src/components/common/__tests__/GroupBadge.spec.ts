import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, named?: Record<string, unknown>) => {
      if (key === 'common.peakRateTooltip') return `peak: ${named?.window}`
      return key
    },
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: { server_utc_offset: '+08:00' } }),
}))

describe('GroupBadge', () => {
  it('shows the peak badge even when rate labels are hidden', () => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: 'Group',
        rateMultiplier: 2,
        showRate: false,
        peakRateEnabled: true,
        peakRateWindows: [{ start: '08:00', end: '09:00', multiplier: 1.5 }],
      },
    })

    expect(wrapper.text()).toContain('08:00-09:00')
    expect(wrapper.text()).not.toContain('2x')
  })
})
