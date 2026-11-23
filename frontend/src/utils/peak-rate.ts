/**
 * 高峰时段倍率的共享展示逻辑。
 *
 * 高峰窗口由后端按服务器全局时区判定（Group.PeakMultiplierAt），
 * 前端展示必须带上服务器时区标注（来自公共设置 server_utc_offset），
 * 避免用户按浏览器本地时间误读计费窗口。
 *
 * 该模块保持纯函数：日期文案通过调用方传入 i18n 的 t，未传入时回退英文，
 * 便于单测/工具脚本在不初始化 vue-i18n 的情况下复用。
 */

/** 0=周日 … 6=周六；空数组/缺省表示每天。 */
export interface PeakRateWindow {
  start: string
  end: string
  multiplier: number
  days?: number[]
}

export interface PeakRateFields {
  peak_rate_enabled?: boolean
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
  peak_rate_windows?: PeakRateWindow[]
}

export type PeakRateTranslator = (key: string, named?: Record<string, unknown>) => string

const FALLBACK_MESSAGES: Record<string, string> = {
  'common.peakRateDays.everyDay': 'Every day',
  'common.peakRateDays.weekdays': 'Weekdays',
  'common.peakRateDays.weekends': 'Weekends',
  'common.peakRateWeekdays.sun': 'Sun',
  'common.peakRateWeekdays.mon': 'Mon',
  'common.peakRateWeekdays.tue': 'Tue',
  'common.peakRateWeekdays.wed': 'Wed',
  'common.peakRateWeekdays.thu': 'Thu',
  'common.peakRateWeekdays.fri': 'Fri',
  'common.peakRateWeekdays.sat': 'Sat',
  'common.peakRateMoreWindows': '{n} more windows',
  'common.peakRateWindowSeparator': '; ',
  'common.peakRateDaySeparator': ', ',
}

function fallbackTranslate(key: string, named?: Record<string, unknown>): string {
  let text = FALLBACK_MESSAGES[key] ?? key
  if (named) {
    for (const [name, value] of Object.entries(named)) {
      text = text.split(`{${name}}`).join(String(value))
    }
  }
  return text
}

const DAY_KEYS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'] as const
/** 展示顺序：周一 … 周日。 */
const DAY_DISPLAY_ORDER = [1, 2, 3, 4, 5, 6, 0] as const

/** HH:MM（00:00–23:59），与后端 parseMinutes / time input 口径一致。 */
export function parsePeakRateMinutes(value: string | null | undefined): number | null {
  if (!value) return null
  const match = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(value.trim())
  if (!match) return null
  return Number(match[1]) * 60 + Number(match[2])
}

/** 单个窗口是否可用于展示/计费：时间格式合法、start < end 且倍率非负。 */
export function isValidPeakRateWindow(window: PeakRateWindow | null | undefined): boolean {
  if (!window) return false
  const start = parsePeakRateMinutes(window.start)
  const end = parsePeakRateMinutes(window.end)
  return (
    start !== null &&
    end !== null &&
    start < end &&
    Number.isFinite(window.multiplier) &&
    window.multiplier >= 0
  )
}

function legacyPeakRateWindow(fields: PeakRateFields): PeakRateWindow | null {
  const candidate: PeakRateWindow = {
    start: fields.peak_start ?? '',
    end: fields.peak_end ?? '',
    multiplier: fields.peak_rate_multiplier ?? 1,
  }
  return isValidPeakRateWindow(candidate) ? candidate : null
}

/**
 * 实际生效的窗口列表。
 * 新数组（peak_rate_windows）非空时以数组为准；数组为空/缺省时回退旧单时段字段。
 * `legacy` 表示当前结果来自旧字段，展示时保持原有格式（不额外加“每天”前缀）。
 */
export function effectivePeakRateWindows(fields?: PeakRateFields | null): {
  windows: PeakRateWindow[]
  legacy: boolean
} {
  if (!fields) return { windows: [], legacy: false }
  const explicit = (fields.peak_rate_windows ?? []).filter(isValidPeakRateWindow)
  if (explicit.length > 0) {
    return { windows: explicit, legacy: false }
  }
  const legacy = legacyPeakRateWindow(fields)
  return { windows: legacy ? [legacy] : [], legacy: Boolean(legacy) }
}

export function hasPeakRate(fields?: PeakRateFields | null): boolean {
  return Boolean(fields?.peak_rate_enabled && effectivePeakRateWindows(fields).windows.length > 0)
}

/** "+08:00" → "UTC+08:00"；旧缓存无该字段时返回空串，调用方降级为不带时区标注 */
export function serverTimezoneLabel(utcOffset?: string | null): string {
  return utcOffset ? `UTC${utcOffset}` : ''
}

function normalizeDays(days?: number[]): number[] {
  if (!days || days.length === 0) return []
  return [...new Set(days.filter((day) => day >= 0 && day <= 6))].sort((a, b) => a - b)
}

function sameDaySet(left: number[], right: number[]): boolean {
  return left.length === right.length && left.every((day, index) => day === right[index])
}

/** 日期集合的展示文案：每天 / 工作日 / 周末 / 周一、周三… */
export function formatPeakRateDays(days?: number[], t?: PeakRateTranslator): string {
  const translate = t ?? fallbackTranslate
  const normalized = normalizeDays(days)
  if (normalized.length === 0 || normalized.length === 7) {
    return translate('common.peakRateDays.everyDay')
  }
  if (sameDaySet(normalized, [1, 2, 3, 4, 5])) {
    return translate('common.peakRateDays.weekdays')
  }
  if (sameDaySet(normalized, [0, 6])) {
    return translate('common.peakRateDays.weekends')
  }
  return DAY_DISPLAY_ORDER.filter((day) => normalized.includes(day))
    .map((day) => translate(`common.peakRateWeekdays.${DAY_KEYS[day]}`))
    .join(translate('common.peakRateDaySeparator'))
}

function formatPeakRateWindowItem(
  window: PeakRateWindow,
  includeDays: boolean,
  t?: PeakRateTranslator
): string {
  const multiplier = Number.isFinite(window.multiplier) ? window.multiplier : 1
  const timeRange = `${window.start}-${window.end} ×${multiplier}`
  return includeDays ? `${formatPeakRateDays(window.days, t)} ${timeRange}` : timeRange
}

/** 窗口列表的完整描述，调用方按空间自行截断；时区标注只加一次。 */
export function formatPeakRateWindow(
  fields: PeakRateFields | null | undefined,
  tzLabel?: string,
  t?: PeakRateTranslator,
  maxWindows?: number
): string {
  if (!hasPeakRate(fields) || !fields) return ''
  const translate = t ?? fallbackTranslate
  const { windows, legacy } = effectivePeakRateWindows(fields)
  const visible =
    maxWindows !== undefined && maxWindows > 0 ? windows.slice(0, maxWindows) : windows
  const parts = visible.map((window) => formatPeakRateWindowItem(window, !legacy, translate))
  if (visible.length < windows.length) {
    parts.push(
      translate('common.peakRateMoreWindows', { n: windows.length - visible.length })
    )
  }
  const base = parts.join(translate('common.peakRateWindowSeparator'))
  return tzLabel ? `${base} (${tzLabel})` : base
}

/** 紧凑场景（徽章/胶囊）使用：只展示前 maxWindows 个，其余以 +N 提示，hover 用完整文案。 */
export function formatPeakRateWindowPreview(
  fields: PeakRateFields | null | undefined,
  tzLabel?: string,
  t?: PeakRateTranslator,
  maxWindows = 1
): string {
  return formatPeakRateWindow(fields, tzLabel, t, maxWindows)
}
