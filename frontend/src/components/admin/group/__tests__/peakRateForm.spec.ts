import { describe, expect, it } from 'vitest'
import {
  buildPeakRateSubmitFields,
  createPeakRateWindowFormEntry,
  formatPeakRateEntryDays,
  peakRateFormEntriesToPayload,
  peakRateWindowsToFormEntries,
  pruneIncompletePeakRateWindows,
} from '../peakRateForm'

describe('peak rate form <-> payload conversion', () => {
  it('backfills legacy single-digit hours and normalizes them for the editor', () => {
    const entries = peakRateWindowsToFormEntries({
      peak_start: '9:00',
      peak_end: '18:00',
      peak_rate_multiplier: 1.5,
    })

    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      start: '09:00',
      end: '18:00',
      multiplier: 1.5,
      days: [],
      daysMode: 'every',
    })
  })

  it('keeps explicit window arrays strict', () => {
    expect(
      peakRateWindowsToFormEntries({
        peak_rate_windows: [{ start: '9:00', end: '10:00', multiplier: 2 }],
      })
    ).toEqual([])
  })

  it('writes two-digit hours when saving the new window array', () => {
    const entries = [
      createPeakRateWindowFormEntry({
        start: '9:00',
        end: '8:30',
        multiplier: 2,
        days: [1, 3],
        daysMode: 'custom',
      }),
    ]

    expect(peakRateFormEntriesToPayload(entries)).toEqual([
      { start: '09:00', end: '08:30', multiplier: 2, days: [1, 3] },
    ])
  })
})

const completeEntry = createPeakRateWindowFormEntry({
  start: '09:00',
  end: '12:00',
  multiplier: 2,
  days: [],
  daysMode: 'every',
})

const blankEntry = createPeakRateWindowFormEntry()

describe('peak rate submit fields', () => {
  it('submits an empty window array when peak rate is disabled', () => {
    expect(buildPeakRateSubmitFields([completeEntry, blankEntry], false)).toEqual({
      peak_rate_windows: [],
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
    })
  })

  it('keeps only complete windows when peak rate is enabled', () => {
    const fields = buildPeakRateSubmitFields([completeEntry, blankEntry], true)

    expect(fields.peak_rate_windows).toEqual([
      { start: '09:00', end: '12:00', multiplier: 2, days: [] },
    ])
    expect(fields.peak_start).toBe('09:00')
    expect(fields.peak_end).toBe('12:00')
    expect(fields.peak_rate_multiplier).toBe(2)
  })

  it('prunes incomplete windows when the toggle is turned off', () => {
    expect(pruneIncompletePeakRateWindows([completeEntry, blankEntry])).toEqual([
      completeEntry,
    ])
  })

  it('seeds one blank window when the toggle is turned on with no windows', () => {
    const seeded = createPeakRateWindowFormEntry()

    expect(seeded).toMatchObject({
      start: '',
      end: '',
      multiplier: 1,
      days: [],
      daysMode: 'every',
    })
  })

  it('does not mutate the legacy payload conversion semantics', () => {
    expect(peakRateFormEntriesToPayload([blankEntry])).toEqual([
      { start: '', end: '', multiplier: 1, days: [] },
    ])
    expect(peakRateFormEntriesToPayload([completeEntry])).toEqual([
      { start: '09:00', end: '12:00', multiplier: 2, days: [] },
    ])
  })
})

describe('peak rate entry day summary', () => {
  const t = (key: string) => key

  it('formats an empty custom day set as unset', () => {
    expect(
      formatPeakRateEntryDays({ days: [], daysMode: 'custom' }, t)
    ).toBe('admin.groups.peakRate.daysUnset')
  })

  it('still formats every-day mode as every day', () => {
    expect(
      formatPeakRateEntryDays({ days: [], daysMode: 'every' }, t)
    ).toBe('common.peakRateDays.everyDay')
  })

  it('formats selected weekdays for a filled custom mode', () => {
    expect(
      formatPeakRateEntryDays({ days: [1, 3], daysMode: 'custom' }, t)
    ).toBe('common.peakRateWeekdays.moncommon.peakRateDaySeparatorcommon.peakRateWeekdays.wed')
  })
})
