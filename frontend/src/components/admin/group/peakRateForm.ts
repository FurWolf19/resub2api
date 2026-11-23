import { isValidPeakRateWindow, type PeakRateWindow } from '@/utils/peak-rate'

export type PeakRateDaysMode = 'every' | 'weekdays' | 'weekends' | 'custom'

export interface PeakRateWindowFormEntry {
  /** 仅用于列表 key，不提交给后端。 */
  id: string
  start: string
  end: string
  multiplier: number | string
  /** 0=周日 … 6=周六；空数组表示每天。 */
  days: number[]
  daysMode: PeakRateDaysMode
  advancedOpen: boolean
}

const WEEKDAYS = [1, 2, 3, 4, 5]
const WEEKENDS = [0, 6]
const ALL_DAYS = [0, 1, 2, 3, 4, 5, 6]

let peakRateWindowSeq = 0

export function createPeakRateWindowFormEntry(
  init: Partial<Omit<PeakRateWindowFormEntry, 'id'>> = {}
): PeakRateWindowFormEntry {
  peakRateWindowSeq += 1
  return {
    id: `peak-rate-window-${peakRateWindowSeq}`,
    start: '',
    end: '',
    multiplier: 1,
    days: [],
    daysMode: 'every',
    advancedOpen: false,
    ...init,
  }
}

function sortedDays(days?: number[]): number[] {
  if (!days || days.length === 0) return []
  return [...new Set(days.filter((day) => day >= 0 && day <= 6))].sort((a, b) => a - b)
}

function sameDaySet(left: number[], right: number[]): boolean {
  return left.length === right.length && left.every((day, index) => day === right[index])
}

export function peakRateDaysModeFromDays(days?: number[]): PeakRateDaysMode {
  const normalized = sortedDays(days)
  if (normalized.length === 0) return 'every'
  if (sameDaySet(normalized, WEEKDAYS)) return 'weekdays'
  if (sameDaySet(normalized, WEEKENDS)) return 'weekends'
  return 'custom'
}

export interface PeakRateSourceFields {
  peak_rate_windows?: PeakRateWindow[]
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
}

/** 回填：优先新数组；数组为空时用旧单时段字段构造一个每天生效的窗口。 */
export function peakRateWindowsToFormEntries(
  fields?: PeakRateSourceFields | null
): PeakRateWindowFormEntry[] {
  if (!fields) return []
  const explicit = (fields.peak_rate_windows ?? []).filter(isValidPeakRateWindow)
  const windows: PeakRateWindow[] =
    explicit.length > 0
      ? explicit
      : isValidPeakRateWindow({
            start: fields.peak_start ?? '',
            end: fields.peak_end ?? '',
            multiplier: fields.peak_rate_multiplier ?? 1,
          })
        ? [
            {
              start: fields.peak_start ?? '',
              end: fields.peak_end ?? '',
              multiplier: fields.peak_rate_multiplier ?? 1,
            },
          ]
        : []

  return windows.map((window) =>
    createPeakRateWindowFormEntry({
      start: window.start,
      end: window.end,
      multiplier: window.multiplier,
      days: sortedDays(window.days),
      daysMode: peakRateDaysModeFromDays(window.days),
    })
  )
}

/** 提交：丢弃 UI 专用字段；每天用空数组表达，其余保持用户选择。 */
export function peakRateFormEntriesToPayload(entries: PeakRateWindowFormEntry[]): PeakRateWindow[] {
  return entries.map((entry) => {
    const multiplier = Number(entry.multiplier)
    return {
      start: entry.start.trim(),
      end: entry.end.trim(),
      multiplier: Number.isFinite(multiplier) ? multiplier : 0,
      days: entry.daysMode === 'every' ? [] : sortedDays(entry.days),
    }
  })
}

/** 自定义模式下若当前为空，给一组可继续调整的默认选中项（每天）。 */
export function defaultCustomDays(): number[] {
  return [...ALL_DAYS]
}
