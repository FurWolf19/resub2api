<template>
  <span v-if="!isApplicable" class="text-sm text-gray-400 dark:text-dark-500">-</span>
  <div v-else class="flex items-center gap-1" data-testid="openai-protocol-capabilities">
    <span
      v-for="badge in badges"
      :key="badge.protocol"
      :class="[
        'inline-flex items-center gap-1 rounded-md px-1.5 py-px text-[10px] font-medium leading-tight',
        badge.colorClass
      ]"
      :title="badge.tooltip"
      :aria-label="badge.ariaLabel"
      :data-protocol="badge.protocol"
      :data-status="badge.status"
      :data-testid="badge.testId"
    >
      <svg
        class="h-2.5 w-2.5 flex-shrink-0"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        aria-hidden="true"
      >
        <path
          v-if="badge.status === 'supported'"
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M4.5 12.75l6 6 9-13.5"
        />
        <path
          v-else-if="badge.status === 'unsupported'"
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M6 18L18 6M6 6l12 12"
        />
        <path
          v-else-if="badge.status === 'forced'"
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 2L4.5 13.5H11l-1 8.5L19.5 10H13V2z"
        />
        <path
          v-else
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z"
        />
      </svg>
      <span>{{ badge.label }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, OpenAIResponsesMode } from '@/types'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

type CapabilityStatus = 'supported' | 'unsupported' | 'unknown'
type BadgeStatus = CapabilityStatus | 'forced'

interface CapabilityView {
  status: CapabilityStatus
  probedAt: string | number | null
}

const STATUS_CLASS: Record<CapabilityStatus, string> = {
  supported: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  unsupported: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  unknown: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
}

const FORCED_CLASS =
  'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'

const STATUS_LABEL_KEY: Record<CapabilityStatus, string> = {
  supported: 'admin.accounts.protocolCapabilities.status.supported',
  unsupported: 'admin.accounts.protocolCapabilities.status.unsupported',
  unknown: 'admin.accounts.protocolCapabilities.status.unknown'
}

const asRecord = (value: unknown): Record<string, unknown> | null =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null

const normalizeMode = (raw: unknown): OpenAIResponsesMode =>
  raw === 'force_responses' || raw === 'force_chat_completions' ? raw : 'auto'

const readProbedAt = (raw: unknown): string | number | null =>
  typeof raw === 'string' || typeof raw === 'number' ? raw : null

// 新键里 supported 不是布尔值时结论不可信，按未知展示。
const parseCapability = (raw: unknown): CapabilityView => {
  const entry = asRecord(raw)
  if (!entry) return { status: 'unknown', probedAt: null }
  const supported = entry.supported
  return {
    status: typeof supported === 'boolean' ? (supported ? 'supported' : 'unsupported') : 'unknown',
    probedAt: readProbedAt(entry.probed_at)
  }
}

const isApplicable = computed(
  () => props.account.platform === 'openai' && props.account.type === 'apikey'
)

// 解析顺序与后端 ResolveProtocolCapabilities 一致：新键 → 旧键 → mode 强制覆盖。
// 旧键只有一个 responses bit：responses 用它；chat_completions 缺失/不可解析时
// 后端按“兼容上游基线能力”判为支持，前端必须同样展示，不能落到 unknown。
const capabilities = computed(() => {
  const extra = asRecord(props.account.extra)
  const rawCapabilities = asRecord(extra?.openai_protocol_capabilities)
  const responses = parseCapability(rawCapabilities?.responses)
  const chatCompletions = parseCapability(rawCapabilities?.chat_completions)

  // 旧键没有 probed_at，不能覆盖新键已有的结论。
  const legacySupported = extra?.openai_responses_supported
  if (typeof legacySupported === 'boolean') {
    if (responses.status === 'unknown') {
      responses.status = legacySupported ? 'supported' : 'unsupported'
    }
    // 与后端一致：旧键存在时 Chat Completions 按支持处理，避免显示“未知”。
    if (chatCompletions.status === 'unknown') {
      chatCompletions.status = 'supported'
    }
  }

  const mode = normalizeMode(extra?.openai_responses_mode)
  // 后端 ResolveResponsesSupport：force_responses / force_chat_completions
  // 直接决定 Responses 路由，auto 才跟随探测结论。
  if (mode === 'force_responses') responses.status = 'supported'
  if (mode === 'force_chat_completions') responses.status = 'unsupported'

  return { responses, chatCompletions, mode }
})

const formatProbedAt = (value: string | number | null): string => {
  if (value === null) return ''
  if (typeof value === 'number') {
    if (!Number.isFinite(value) || value <= 0) return ''
    // 后端按 Unix 秒解析；兼容误写成毫秒的历史数据。
    return formatDateTime(new Date(value > 1e12 ? value : value * 1000))
  }
  const text = value.trim()
  if (!text) return ''
  if (/^\d+$/.test(text)) {
    const seconds = Number(text)
    if (!Number.isFinite(seconds) || seconds <= 0) return ''
    return formatDateTime(new Date(seconds > 1e12 ? seconds : seconds * 1000))
  }
  return formatDateTime(text)
}

const buildTooltip = (label: string, capability: CapabilityView): string => {
  const parts = [`${label} · ${t(STATUS_LABEL_KEY[capability.status])}`]
  const probedAt = formatProbedAt(capability.probedAt)
  if (probedAt) {
    parts.push(t('admin.accounts.protocolCapabilities.probedAt', { time: probedAt }))
  }
  return parts.join('\n')
}

const buildStatusLabel = (label: string, status: CapabilityStatus): string =>
  `${label} · ${t(STATUS_LABEL_KEY[status])}`

// 强制模式下协议由账号配置直接决定，探测结论不参与路由，因此只展示一枚
// 紫色强制徽标，不再展示 Chat/Responses 的绿/红/灰状态，避免误读。
const badges = computed(() => {
  const { responses, chatCompletions, mode } = capabilities.value
  if (mode !== 'auto') {
    const forcedResponses = mode === 'force_responses'
    const label = forcedResponses
      ? t('admin.accounts.protocolCapabilities.forcedResponses')
      : t('admin.accounts.protocolCapabilities.forcedChat')
    const forcedStatus: BadgeStatus = 'forced'
    return [
      {
        protocol: forcedResponses ? ('responses' as const) : ('chat_completions' as const),
        label,
        status: forcedStatus,
        colorClass: FORCED_CLASS,
        ariaLabel: `${t('admin.accounts.protocolCapabilities.force')} · ${label}`,
        tooltip: [
          label,
          t('admin.accounts.protocolCapabilities.forcedOverrideHint')
        ].join('\n'),
        testId: 'protocol-badge-forced'
      }
    ]
  }

  const chatLabel = t('admin.accounts.protocolCapabilities.chatCompletions')
  const responsesLabel = t('admin.accounts.protocolCapabilities.responses')
  return [
    {
      protocol: 'chat_completions' as const,
      label: t('admin.accounts.protocolCapabilities.chat'),
      status: chatCompletions.status,
      colorClass: STATUS_CLASS[chatCompletions.status],
      ariaLabel: buildStatusLabel(chatLabel, chatCompletions.status),
      tooltip: buildTooltip(chatLabel, chatCompletions),
      testId: 'protocol-badge-chat_completions'
    },
    {
      protocol: 'responses' as const,
      label: t('admin.accounts.protocolCapabilities.responses'),
      status: responses.status,
      colorClass: STATUS_CLASS[responses.status],
      ariaLabel: buildStatusLabel(responsesLabel, responses.status),
      tooltip: buildTooltip(responsesLabel, responses),
      testId: 'protocol-badge-responses'
    }
  ]
})
</script>
