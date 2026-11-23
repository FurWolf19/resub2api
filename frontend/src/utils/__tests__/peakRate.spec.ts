import { describe, expect, it } from 'vitest'
import {
  effectivePeakRateWindows,
  formatPeakRateDays,
  formatPeakRateWindow,
  formatPeakRateWindowPreview,
  hasPeakRate,
  normalizePeakRateTime,
  parsePeakRateMinutes,
  type PeakRateFields,
} from '../peak-rate'

const labels: Record<string, string> = {
  'common.peakRateDays.everyDay': '每天',
  'common.peakRateDays.weekdays': '工作日',
  'common.peakRateDays.weekends': '周末',
  'common.peakRateWeekdays.mon': '周一',
  'common.peakRateWeekdays.wed': '周三',
  'common.peakRateMoreWindows': '另有 {n} 个时段',
  'common.peakRateWindowSeparator': '；',
  'common.peakRateDaySeparator': '、',
}
const t = (key: string, named?: Record<string, unknown>) => {
  let text = labels[key] ?? key
  for (const [name, value] of Object.entries(named ?? {})) {
    text = text.split(`{${name}}`).join(String(value))
  }
  return text
}

describe('peak-rate display helpers', () => {
  it('falls back to legacy single-window fields', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_start: '14:00',
      peak_end: '18:00',
      peak_rate_multiplier: 1.5,
    }
    expect(hasPeakRate(fields)).toBe(true)
    expect(effectivePeakRateWindows(fields).legacy).toBe(true)
    expect(formatPeakRateWindow(fields, 'UTC+08:00')).toBe('14:00-18:00 ×1.5 (UTC+08:00)')
  })

  it('formats multiple windows with day labels and one timezone suffix', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_rate_windows: [
        { start: '12:00', end: '14:00', multiplier: 1.5, days: [1, 2, 3, 4, 5] },
        { start: '20:00', end: '22:00', multiplier: 2, days: [0, 6] },
      ],
    }
    expect(formatPeakRateWindow(fields, 'UTC+08:00', t)).toBe(
      '工作日 12:00-14:00 ×1.5；周末 20:00-22:00 ×2 (UTC+08:00)'
    )
  })

  it('shows +N in previews and keeps full text available', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_rate_windows: [
        { start: '08:00', end: '09:00', multiplier: 1.1 },
        { start: '12:00', end: '13:00', multiplier: 1.2 },
      ],
    }
    expect(formatPeakRateWindowPreview(fields, '', t)).toBe(
      '每天 08:00-09:00 ×1.1；另有 1 个时段'
    )
    expect(formatPeakRateWindow(fields, '', t)).toContain('每天 12:00-13:00 ×1.2')
  })

  it('treats empty days as every day and custom days with Chinese separators', () => {
    expect(formatPeakRateDays([], t)).toBe('每天')
    expect(formatPeakRateDays([1, 3], t)).toBe('周一、周三')
  })

  it('accepts and normalizes legacy single-digit hours', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_start: '9:00',
      peak_end: '18:00',
      peak_rate_multiplier: 1.5,
    }
    expect(normalizePeakRateTime('9:00')).toBe('09:00')
    expect(hasPeakRate(fields)).toBe(true)
    expect(effectivePeakRateWindows(fields).windows[0]).toMatchObject({
      start: '09:00',
      end: '18:00',
    })
    expect(formatPeakRateWindow(fields)).toBe('09:00-18:00 ×1.5')
  })

  it('keeps explicit window arrays strict', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_rate_windows: [{ start: '9:00', end: '10:00', multiplier: 2 }],
    }
    expect(parsePeakRateMinutes('9:00')).toBeNull()
    expect(hasPeakRate(fields)).toBe(false)
  })

  it('ignores invalid windows and requires enabled', () => {
    const fields: PeakRateFields = {
      peak_rate_enabled: true,
      peak_rate_windows: [{ start: '22:00', end: '02:00', multiplier: 2 }],
    }
    expect(hasPeakRate(fields)).toBe(false)
    expect(formatPeakRateWindow(fields)).toBe('')
    expect(hasPeakRate({ ...fields, peak_rate_enabled: false })).toBe(false)
  })
})
