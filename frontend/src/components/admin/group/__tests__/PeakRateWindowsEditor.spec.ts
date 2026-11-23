import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import PeakRateWindowsEditor from '../PeakRateWindowsEditor.vue'
import { createPeakRateWindowFormEntry } from '../peakRateForm'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function mountEditor(overrides: Parameters<typeof createPeakRateWindowFormEntry>[0]) {
  return mount(PeakRateWindowsEditor, {
    props: {
      modelValue: [createPeakRateWindowFormEntry(overrides)]
    },
    global: {
      stubs: {
        Icon: true
      }
    }
  })
}

function summaryText(wrapper: ReturnType<typeof mountEditor>) {
  return wrapper.get('button[aria-expanded]').text()
}

describe('PeakRateWindowsEditor day summary', () => {
  it('shows not-set instead of every-day when custom days are empty', () => {
    const wrapper = mountEditor({ daysMode: 'custom', days: [], advancedOpen: true })
    const text = summaryText(wrapper)

    expect(text).toContain('admin.groups.peakRate.daysUnset')
    expect(text).not.toContain('common.peakRateDays.everyDay')
    expect(wrapper.text()).toContain('admin.groups.peakRate.customDaysRequired')
  })

  it('still shows every-day for the every-day mode', () => {
    const text = summaryText(mountEditor({ daysMode: 'every', days: [] }))

    expect(text).toContain('common.peakRateDays.everyDay')
    expect(text).not.toContain('admin.groups.peakRate.daysUnset')
  })

  it('shows selected weekdays for a filled custom mode', () => {
    const text = summaryText(mountEditor({ daysMode: 'custom', days: [1, 3] }))

    expect(text).toContain('common.peakRateWeekdays.mon')
    expect(text).toContain('common.peakRateWeekdays.wed')
    expect(text).not.toContain('admin.groups.peakRate.daysUnset')
  })
})
