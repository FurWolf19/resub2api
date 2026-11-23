<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-end justify-between gap-2">
      <div class="min-w-0">
        <div class="flex items-center gap-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.groups.peakRate.windowsTitle') }}
          </span>
          <span
            v-if="timezoneLabel"
            class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-300"
          >
            {{ t('admin.groups.peakRate.serverTimezone', { timezone: timezoneLabel }) }}
          </span>
        </div>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.peakRate.windowsHint') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-secondary shrink-0 whitespace-nowrap"
        @click="addWindow"
      >
        <Icon name="plus" size="sm" class="mr-1" />
        {{ t('admin.groups.peakRate.addWindow') }}
      </button>
    </div>

    <div
      v-if="modelValue.length === 0"
      class="rounded-lg border border-dashed border-gray-200 bg-gray-50/60 px-4 py-6 text-center text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800/40 dark:text-dark-400"
    >
      {{ t('admin.groups.peakRate.emptyHint') }}
    </div>

    <div v-else class="space-y-3">
      <div
        v-for="(entry, index) in modelValue"
        :key="entry.id"
        class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800/60 sm:p-4"
      >
        <div class="flex flex-wrap items-end gap-2 sm:gap-3">
          <div class="min-w-[7.5rem] flex-1">
            <label class="input-label">
              {{ t('admin.groups.peakRate.startTime') }}
            </label>
            <input
              :value="entry.start"
              type="time"
              step="60"
              class="input"
              :aria-label="t('admin.groups.peakRate.startTime')"
              @input="updateEntry(index, { start: ($event.target as HTMLInputElement).value })"
            />
          </div>
          <div class="min-w-[7.5rem] flex-1">
            <label class="input-label">
              {{ t('admin.groups.peakRate.endTime') }}
            </label>
            <input
              :value="entry.end"
              type="time"
              step="60"
              class="input"
              :aria-label="t('admin.groups.peakRate.endTime')"
              @input="updateEntry(index, { end: ($event.target as HTMLInputElement).value })"
            />
          </div>
          <div class="w-28 shrink-0">
            <label class="input-label">
              {{ t('admin.groups.peakRate.peakMultiplier') }}
            </label>
            <input
              :value="entry.multiplier"
              type="number"
              min="0"
              step="0.1"
              class="input"
              placeholder="1"
              :title="t('admin.groups.peakRate.multiplierHint')"
              :aria-label="t('admin.groups.peakRate.peakMultiplier')"
              @input="updateEntry(index, { multiplier: ($event.target as HTMLInputElement).value })"
            />
          </div>
          <button
            type="button"
            class="mb-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            :title="t('admin.groups.peakRate.removeWindow')"
            :aria-label="t('admin.groups.peakRate.removeWindow')"
            @click="removeWindow(index)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>

        <div class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700/70">
          <button
            type="button"
            class="flex max-w-full items-center gap-1.5 text-left text-xs font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
            :aria-expanded="entry.advancedOpen"
            @click="updateEntry(index, { advancedOpen: !entry.advancedOpen })"
          >
            <Icon
              name="chevronDown"
              size="xs"
              class="shrink-0 transition-transform duration-200"
              :class="{ 'rotate-180': entry.advancedOpen }"
            />
            <span>{{ t('admin.groups.peakRate.advanced') }}</span>
            <span class="truncate font-normal text-gray-400 dark:text-dark-500">
              {{ formatPeakRateDays(entry.days, t) }}
            </span>
          </button>

          <div
            class="overflow-hidden transition-all duration-200 ease-in-out"
            :class="entry.advancedOpen ? 'mt-3 max-h-80 opacity-100' : 'max-h-0 opacity-0'"
          >
            <div class="rounded-lg border border-gray-200 bg-gray-50/80 p-3 dark:border-dark-700 dark:bg-dark-900/40">
              <div class="text-xs font-medium text-gray-600 dark:text-dark-300">
                {{ t('admin.groups.peakRate.effectiveDays') }}
              </div>
              <div class="mt-2 flex flex-wrap gap-1.5">
                <button
                  v-for="preset in dayPresets"
                  :key="preset.value"
                  type="button"
                  class="rounded-full border px-2.5 py-1 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
                  :class="
                    entry.daysMode === preset.value
                      ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-300'
                      : 'border-gray-300 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'
                  "
                  @click="setDaysMode(index, preset.value)"
                >
                  {{ preset.label }}
                </button>
              </div>

              <div
                v-if="entry.daysMode === 'custom'"
                class="mt-3 flex flex-wrap gap-1.5"
              >
                <label
                  v-for="day in weekdayOptions"
                  :key="day.value"
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors"
                  :class="
                    entry.days.includes(day.value)
                      ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-500/60 dark:bg-primary-900/30 dark:text-primary-300'
                      : 'border-gray-300 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'
                  "
                >
                  <input
                    type="checkbox"
                    class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700"
                    :checked="entry.days.includes(day.value)"
                    @change="toggleDay(index, day.value)"
                  />
                  <span>{{ day.label }}</span>
                </label>
              </div>
              <p
                v-if="entry.daysMode === 'custom' && entry.days.length === 0"
                class="mt-2 text-xs text-red-500 dark:text-red-400"
              >
                {{ t('admin.groups.peakRate.customDaysRequired') }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatPeakRateDays } from '@/utils/peak-rate'
import {
  createPeakRateWindowFormEntry,
  defaultCustomDays,
  type PeakRateDaysMode,
  type PeakRateWindowFormEntry,
} from './peakRateForm'

const props = defineProps<{
  modelValue: PeakRateWindowFormEntry[]
  timezoneLabel?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: PeakRateWindowFormEntry[]]
}>()

const { t } = useI18n()

const dayPresets = computed<Array<{ value: PeakRateDaysMode; label: string }>>(() => [
  { value: 'every', label: t('admin.groups.peakRate.everyDay') },
  { value: 'weekdays', label: t('admin.groups.peakRate.weekdaysOnly') },
  { value: 'weekends', label: t('admin.groups.peakRate.weekendsOnly') },
  { value: 'custom', label: t('admin.groups.peakRate.custom') },
])

const weekdayOptions = computed(() => [
  { value: 1, label: t('common.peakRateWeekdays.mon') },
  { value: 2, label: t('common.peakRateWeekdays.tue') },
  { value: 3, label: t('common.peakRateWeekdays.wed') },
  { value: 4, label: t('common.peakRateWeekdays.thu') },
  { value: 5, label: t('common.peakRateWeekdays.fri') },
  { value: 6, label: t('common.peakRateWeekdays.sat') },
  { value: 0, label: t('common.peakRateWeekdays.sun') },
])

function updateEntry(index: number, patch: Partial<PeakRateWindowFormEntry>) {
  emit(
    'update:modelValue',
    props.modelValue.map((entry, current) =>
      current === index ? { ...entry, ...patch } : entry
    )
  )
}

function addWindow() {
  emit('update:modelValue', [...props.modelValue, createPeakRateWindowFormEntry()])
}

function removeWindow(index: number) {
  emit(
    'update:modelValue',
    props.modelValue.filter((_entry, current) => current !== index)
  )
}

function setDaysMode(index: number, mode: PeakRateDaysMode) {
  const entry = props.modelValue[index]
  if (!entry) return
  let days = entry.days
  if (mode === 'every') {
    days = []
  } else if (mode === 'weekdays') {
    days = [1, 2, 3, 4, 5]
  } else if (mode === 'weekends') {
    days = [0, 6]
  } else if (days.length === 0) {
    // 从“每天”切到自定义时给出全部星期，用户再按需取消，避免出现空选。
    days = defaultCustomDays()
  }
  updateEntry(index, { daysMode: mode, days })
}

function toggleDay(index: number, day: number) {
  const entry = props.modelValue[index]
  if (!entry) return
  const days = entry.days.includes(day)
    ? entry.days.filter((value) => value !== day)
    : [...entry.days, day].sort((a, b) => a - b)
  updateEntry(index, { daysMode: 'custom', days })
}

function parseMinutes(value: string): number | null {
  const match = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(value.trim())
  if (!match) return null
  return Number(match[1]) * 60 + Number(match[2])
}

/** 保存前校验；返回本地化错误文案，null 表示通过。后端仍会做最终校验。 */
function validate(): string | null {
  if (props.modelValue.length === 0) {
    return t('admin.groups.peakRate.emptyError')
  }

  const spans: Array<{ start: number; end: number; window: number; day: number }> = []
  for (let index = 0; index < props.modelValue.length; index += 1) {
    const entry = props.modelValue[index]
    const position = index + 1
    if (!entry.start) {
      return t('admin.groups.peakRate.startRequired', { index: position })
    }
    if (!entry.end) {
      return t('admin.groups.peakRate.endRequired', { index: position })
    }
    const start = parseMinutes(entry.start)
    if (start === null) {
      return t('admin.groups.peakRate.startRequired', { index: position })
    }
    const end = parseMinutes(entry.end)
    if (end === null) {
      return t('admin.groups.peakRate.endRequired', { index: position })
    }
    if (start >= end) {
      return t('admin.groups.peakRate.invalidRange', { index: position })
    }
    if (String(entry.multiplier).trim() === '') {
      return t('admin.groups.peakRate.multiplierRequired', { index: position })
    }
    const multiplier = Number(entry.multiplier)
    if (!Number.isFinite(multiplier) || multiplier < 0) {
      return t('admin.groups.peakRate.invalidMultiplier', { index: position })
    }
    if (entry.daysMode === 'custom' && entry.days.length === 0) {
      return t('admin.groups.peakRate.customDaysRequired')
    }

    const days = entry.days.length === 0 ? [0, 1, 2, 3, 4, 5, 6] : entry.days
    for (const day of days) {
      spans.push({ start: day * 1440 + start, end: day * 1440 + end, window: index, day })
    }
  }

  spans.sort((left, right) =>
    left.start === right.start ? left.end - right.end : left.start - right.start
  )
  let maxEndIndex = -1
  for (let index = 0; index < spans.length; index += 1) {
    if (maxEndIndex >= 0 && spans[index].start < spans[maxEndIndex].end) {
      const previous = spans[maxEndIndex]
      const current = spans[index]
      return t('admin.groups.peakRate.overlapError', {
        first: previous.window + 1,
        second: current.window + 1,
        day: formatPeakRateDays([current.day], t),
      })
    }
    if (maxEndIndex < 0 || spans[index].end > spans[maxEndIndex].end) {
      maxEndIndex = index
    }
  }
  return null
}

defineExpose({ validate })
</script>
