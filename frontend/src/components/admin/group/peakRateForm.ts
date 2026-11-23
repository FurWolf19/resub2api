import {
  effectivePeakRateWindows,
  formatPeakRateDays,
  isValidPeakRateWindow,
  normalizePeakRateTime,
  type PeakRateTranslator,
  type PeakRateWindow,
} from '@/utils/peak-rate'

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
  if (normalized.length === 0 || normalized.length === 7) return 'every'
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

/**
 * 回填：优先新数组；数组为空时用旧单时段字段构造一个每天生效的窗口。
 * 旧字段允许 `9:00` 这类单位数小时写法，由共享解析器规范成 `09:00`。
 */
export function peakRateWindowsToFormEntries(
  fields?: PeakRateSourceFields | null
): PeakRateWindowFormEntry[] {
  if (!fields) return []
  const { windows } = effectivePeakRateWindows(fields)

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

/** 提交：丢弃 UI 专用字段；时间统一规范成两位小时，每天用空数组表达。 */
export function peakRateFormEntriesToPayload(entries: PeakRateWindowFormEntry[]): PeakRateWindow[] {
  return entries.map(peakRateEntryToWindow)
}

function peakRateEntryToWindow(entry: PeakRateWindowFormEntry): PeakRateWindow {
  const multiplier = Number(entry.multiplier)
  return {
    start: normalizePeakRateTime(entry.start) ?? entry.start.trim(),
    end: normalizePeakRateTime(entry.end) ?? entry.end.trim(),
    multiplier: Number.isFinite(multiplier) ? multiplier : 0,
    days: entry.daysMode === 'every' ? [] : sortedDays(entry.days),
  }
}

/** 关闭开关时保留填完整窗口的草稿，丢弃空白/非法条目。 */
export function pruneIncompletePeakRateWindows(
  entries: PeakRateWindowFormEntry[]
): PeakRateWindowFormEntry[] {
  return entries.filter((entry) => {
    if (String(entry.multiplier).trim() === '') return false
    return isValidPeakRateWindow(peakRateEntryToWindow(entry))
  })
}

/**
 * 保存载荷：关闭时不提交任何窗口；开启时只提交完整窗口，并同步旧单时段字段。
 */
export function buildPeakRateSubmitFields(
  entries: PeakRateWindowFormEntry[],
  enabled: boolean
): {
  peak_rate_windows: PeakRateWindow[]
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
} {
  if (!enabled) {
    return {
      peak_rate_windows: [],
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
    }
  }
  const windows = pruneIncompletePeakRateWindows(entries).map(peakRateEntryToWindow)
  const first = windows[0]
  return {
    peak_rate_windows: windows,
    peak_start: first?.start ?? '',
    peak_end: first?.end ?? '',
    peak_rate_multiplier: first?.multiplier ?? 1,
  }
}

/** 编辑器摘要：自定义模式空选时显示“未设置”，避免误读成每天生效。 */
export function formatPeakRateEntryDays(
  entry: Pick<PeakRateWindowFormEntry, 'days' | 'daysMode'>,
  t: PeakRateTranslator
): string {
  if (entry.daysMode === 'custom' && entry.days.length === 0) {
    return t('admin.groups.peakRate.daysUnset')
  }
  return formatPeakRateDays(entry.days, t)
}

/** 自定义模式下若当前为空，给一组可继续调整的默认选中项（每天）。 */
export function defaultCustomDays(): number[] {
  return [...ALL_DAYS]
}
