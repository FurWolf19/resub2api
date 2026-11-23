<template>
  <BaseDialog :show="show" :title="t('admin.groups.rateMultipliersTitle')" width="wide" @close="handleClose">
    <div v-if="group" class="space-y-4">
      <!-- 分组信息 -->
      <div class="flex flex-wrap items-center gap-3 rounded-lg bg-gray-50 px-4 py-2.5 text-sm dark:bg-dark-700">
        <span class="inline-flex items-center gap-1.5" :class="platformColorClass">
          <PlatformIcon :platform="group.platform" size="sm" />
          {{ t('admin.groups.platforms.' + group.platform, platformLabel(group.platform)) }}
        </span>
        <span class="text-gray-400">|</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
        <span class="text-gray-400">|</span>
        <span class="text-gray-600 dark:text-gray-400">
          {{ t('admin.groups.columns.rateMultiplier') }}: {{ group.rate_multiplier }}x
        </span>
      </div>

      <!-- 操作区 -->
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <!-- 添加用户 -->
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.addUserRate') }}
        </h4>
        <div class="flex items-end gap-2">
          <div class="relative flex-1">
            <input
              v-model="searchQuery"
              type="text"
              autocomplete="off"
              class="input w-full"
              :placeholder="t('admin.groups.searchUserPlaceholder')"
              @input="handleSearchUsers"
              @focus="showDropdown = true"
            />
            <div
              v-if="showDropdown && searchResults.length > 0"
              class="absolute left-0 right-0 top-full z-10 mt-1 max-h-48 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-500 dark:bg-dark-700"
            >
              <button
                v-for="user in searchResults"
                :key="user.id"
                type="button"
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
                @click="selectUser(user)"
              >
                <span class="text-gray-400">#{{ user.id }}</span>
                <span class="text-gray-900 dark:text-white">{{ user.username || user.email }}</span>
                <span v-if="user.username" class="text-xs text-gray-400">{{ user.email }}</span>
              </button>
            </div>
          </div>
          <div class="w-24">
            <input
              v-model.number="newRate"
              type="number"
              step="0.001"
              min="0"
              autocomplete="off"
              class="hide-spinner input w-full"
              placeholder="1.0"
            />
          </div>
          <button
            type="button"
            class="btn btn-primary shrink-0"
            :disabled="!selectedUser || newRate == null || newRate <= 0"
            @click="handleAddLocal"
          >
            {{ t('common.add') }}
          </button>
        </div>

        <!-- 批量调整 + 全部清空 -->
        <div v-if="localEntries.length > 0" class="mt-3 space-y-2 border-t border-gray-100 pt-3 dark:border-dark-600">
          <div class="flex flex-wrap items-center gap-3">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.batchAdjust') }}</span>
            <div class="flex items-center gap-1.5">
              <span class="text-xs text-gray-400">×</span>
              <input
                v-model.number="batchFactor"
                type="number"
                step="0.1"
                min="0"
                autocomplete="off"
                class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
                placeholder="0.5"
              />
              <button
                type="button"
                class="btn btn-primary btn-sm shrink-0 px-2.5 py-1 text-xs"
                :disabled="!batchFactor || batchFactor <= 0"
                @click="applyBatchFactor"
              >
                {{ t('admin.groups.applyMultiplier') }}
              </button>
            </div>
            <div class="ml-auto">
              <button
                type="button"
                class="rounded-lg border border-red-200 bg-red-50 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-100 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/40"
                @click="requestClearAllLocal"
              >
                {{ t('admin.groups.clearAll') }}
              </button>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.batchQuotaMultiplier') }}</span>
            <input
              v-model.number="batchQuotaMultiplier"
              type="number"
              step="0.05"
              min="0"
              max="1"
              autocomplete="off"
              :disabled="quotaLoadFailed"
              class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
              :placeholder="t('admin.groups.quotaMultiplierPlaceholder')"
            />
            <span class="w-12 text-xs tabular-nums text-gray-400">{{ formatQuotaMultiplier(batchQuotaMultiplier) }}</span>
            <button
              type="button"
              class="btn btn-primary btn-sm shrink-0 px-2.5 py-1 text-xs"
              :disabled="quotaLoadFailed || parseQuotaMultiplier(batchQuotaMultiplier) == null"
              @click="applyBatchQuotaMultiplier"
            >
              {{ t('admin.groups.applyQuotaMultiplier') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="loading" class="flex justify-center py-6">
        <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <!-- 已设置的用户列表 -->
      <div v-else>
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.rateMultipliers') }} ({{ rateEntryCount }})
        </h4>

        <!-- 额度倍率接口不可用时只禁用额度列，价格倍率仍可管理 -->
        <div
          v-if="quotaLoadFailed"
          class="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-300"
          role="status"
        >
          <span>{{ t('admin.groups.quotaMultiplierLoadFailed') }}</span>
          <button
            type="button"
            class="shrink-0 rounded px-1.5 py-0.5 font-medium underline underline-offset-2 transition-colors hover:bg-amber-100 dark:hover:bg-amber-900/40"
            :disabled="loading"
            @click="loadEntries"
          >
            {{ t('admin.groups.retryLoad') }}
          </button>
        </div>

        <div v-if="localEntries.length === 0" class="py-6 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ t('admin.groups.noRateMultipliers') }}
        </div>

        <div v-else>
          <!-- 表格 -->
          <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <div class="max-h-[420px] overflow-auto">
              <table class="w-full min-w-max text-sm">
                <thead class="sticky top-0 z-[1]">
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-700">
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userEmail') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">ID</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userName') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userNotes') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userStatus') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.rateMultiplier') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.quotaMultiplier') }}</th>
                    <th v-if="showFinalRate" class="px-3 py-2 text-left text-xs font-medium text-primary-600 dark:text-primary-400">{{ t('admin.groups.finalRate') }}</th>
                    <th class="w-10 px-2 py-2"></th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                  <tr
                    v-for="entry in paginatedLocalEntries"
                    :key="entry.user_id"
                    class="hover:bg-gray-50 dark:hover:bg-dark-700/50"
                  >
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ entry.user_email }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-400 dark:text-gray-500">{{ entry.user_id }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-900 dark:text-white">{{ entry.user_name || '-' }}</td>
                    <td class="max-w-[160px] truncate px-3 py-2 text-gray-500 dark:text-gray-400" :title="entry.user_notes">{{ entry.user_notes || '-' }}</td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <span
                        :class="[
                          'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
                          entry.user_status === 'active'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
                        ]"
                      >
                        {{ entry.user_status }}
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <input
                        type="number"
                        step="0.001"
                        min="0.001"
                        autocomplete="off"
                        :value="entry.rate_multiplier ?? ''"
                        :placeholder="String(props.group?.rate_multiplier ?? 1)"
                        class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm font-medium transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
                        @change="updateLocalRate(entry.user_id, ($event.target as HTMLInputElement).value)"
                      />
                    </td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <div class="flex items-center gap-2">
                        <input
                          type="number"
                          step="0.05"
                          min="0"
                          max="1"
                          autocomplete="off"
                          :value="quotaValue(entry) ?? ''"
                          :placeholder="
                            entry.subscription_id == null
                              ? t('admin.groups.quotaMultiplierUnset')
                              : undefined
                          "
                          :disabled="quotaLoadFailed"
                          class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm font-medium transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
                          :title="
                            quotaLoadFailed
                              ? t('admin.groups.quotaMultiplierLoadFailed')
                              : entry.subscription_id == null
                                ? t('admin.groups.quotaMultiplierDefaultHint')
                                : t('admin.groups.quotaMultiplierClearHint')
                          "
                          @change="updateLocalQuotaMultiplier(entry.user_id, ($event.target as HTMLInputElement).value, $event.target as HTMLInputElement)"
                        />
                        <span class="w-12 text-right text-xs tabular-nums text-gray-400 dark:text-gray-500">
                          {{ formatQuotaMultiplier(quotaValue(entry)) }}
                        </span>
                        <span
                          v-if="entry.quota_multiplier != null"
                          class="shrink-0 whitespace-nowrap rounded-full bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
                        >
                          {{ t('admin.groups.quotaMultiplierEffectiveBadge') }}
                        </span>
                        <span
                          v-else-if="entry.default_quota_multiplier != null && entry.subscription_id == null"
                          class="shrink-0 whitespace-nowrap rounded-full bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
                        >
                          {{ t('admin.groups.quotaMultiplierDefaultBadge') }}
                        </span>
                      </div>
                    </td>
                    <td v-if="showFinalRate" class="whitespace-nowrap px-3 py-2 font-medium text-primary-600 dark:text-primary-400">
                      {{ computeFinalRate(entry.rate_multiplier) }}
                    </td>
                    <td class="px-2 py-2">
                      <button
                        v-if="entry.rate_multiplier != null"
                        type="button"
                        class="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        @click="removeLocal(entry.user_id)"
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- 分页 -->
          <Pagination
            :total="localEntries.length"
            :page="currentPage"
            :page-size="pageSize"
            @update:page="currentPage = $event"
            @update:pageSize="handlePageSizeChange"
          />
        </div>
      </div>

      <!-- 底部操作栏 -->
      <div class="flex items-center gap-3 border-t border-gray-200 pt-4 dark:border-dark-600">
        <!-- 左侧：未保存提示 + 撤销 -->
        <template v-if="isDirty">
          <span class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.groups.unsavedChanges') }}</span>
          <button
            type="button"
            class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            @click="handleCancel"
          >
            {{ t('admin.groups.revertChanges') }}
          </button>
        </template>
        <!-- 右侧：关闭 / 保存 -->
        <div class="ml-auto flex items-center gap-3">
          <button type="button" class="btn btn-sm px-4 py-1.5" @click="handleClose">
            {{ t('common.close') }}
          </button>
          <button
            v-if="isDirty"
            type="button"
            class="btn btn-primary btn-sm px-4 py-1.5"
            :disabled="saving"
            @click="handleSave"
          >
            <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="showClearAllConfirm"
    :title="t('admin.groups.clearAll')"
    :message="t('admin.groups.confirmClearAll')"
    :confirm-text="t('admin.groups.clearAll')"
    danger
    @confirm="confirmClearAllLocal"
    @cancel="showClearAllConfirm = false"
  />
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  GroupQuotaMultiplierEntry,
  GroupRateMultiplierEntry
} from '@/api/admin/groups'
import type { AdminGroup, AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { platformLabel } from '@/utils/platformColors'

interface LocalEntry extends GroupRateMultiplierEntry {
  subscription_id?: number | null
  quota_multiplier: number | null
  default_quota_multiplier?: number | null
}

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const serverEntries = ref<LocalEntry[]>([])
const localEntries = ref<LocalEntry[]>([])
const searchQuery = ref('')
const searchResults = ref<AdminUser[]>([])
const showDropdown = ref(false)
const selectedUser = ref<AdminUser | null>(null)
const newRate = ref<number | null>(null)
const currentPage = ref(1)
const pageSize = ref(10)
const batchFactor = ref<number | null>(null)
const batchQuotaMultiplier = ref<number | null>(null)
const showClearAllConfirm = ref(false)
/** 额度倍率接口不可用：只禁用额度列，价格倍率仍可编辑。 */
const quotaLoadFailed = ref(false)
/** 当前已成功加载的分组，用于区分“切换分组”和“同组重载”。 */
const loadedGroupId = ref<number | null>(null)
/** 本地删除的、需要额外提交 quota_multiplier: null 清除额度配置的用户。 */
const removedUserIds = ref<number[]>([])

let searchTimeout: ReturnType<typeof setTimeout>
let loadVersion = 0

const platformColorClass = computed(() => {
  switch (props.group?.platform) {
    case 'anthropic': return 'text-orange-700 dark:text-orange-400'
    case 'openai': return 'text-emerald-700 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-700 dark:text-purple-400'
    default: return 'text-blue-700 dark:text-blue-400'
  }
})

// 是否显示"最终倍率"预览列
const showFinalRate = computed(() => {
  return batchFactor.value != null && batchFactor.value > 0 && batchFactor.value !== 1
})

// 计算最终倍率预览
const computeFinalRate = (rate: number | null | undefined) => {
  const base = rate ?? props.group?.rate_multiplier ?? 1
  if (!batchFactor.value) return base
  return parseFloat((base * batchFactor.value).toFixed(6))
}

const formatQuotaMultiplier = (value: number | null | undefined): string => {
  if (value == null) return '—'
  return `${(value * 100).toFixed(1)}%`
}

/**
 * 额度输入框展示/提交的值：有生效订阅只读订阅额度（清空=1.0），
 * 无订阅只读下次分配默认值。两者不互相回退，避免默认值被误标成“生效”。
 */
const quotaValue = (
  entry: Pick<LocalEntry, 'subscription_id' | 'quota_multiplier' | 'default_quota_multiplier'>
) => {
  if (entry.subscription_id == null) {
    return entry.default_quota_multiplier ?? null
  }
  return entry.quota_multiplier ?? 1
}

/** 标题计数只统计价格倍率用户；表格仍展示仅额度用户。 */
const rateEntryCount = computed(
  () => localEntries.value.filter((entry) => entry.rate_multiplier != null).length
)

const parseQuotaMultiplier = (value: number | string | null | undefined): number | null => {
  if (value === null || value === undefined || value === '') return null
  const numeric = Number(value)
  if (!Number.isFinite(numeric) || numeric < 0 || numeric > 1) return null
  return numeric
}

// 检测是否有未保存的修改
const isDirty = computed(() => {
  if (localEntries.value.length !== serverEntries.value.length) return true
  const serverMap = new Map(
    serverEntries.value.map(e => [
      e.user_id,
      { rate: e.rate_multiplier ?? null, quota: quotaValue(e) }
    ])
  )
  return localEntries.value.some(e => {
    const server = serverMap.get(e.user_id)
    return (
      !server ||
      server.rate !== (e.rate_multiplier ?? null) ||
      server.quota !== quotaValue(e)
    )
  })
})

const paginatedLocalEntries = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return localEntries.value.slice(start, start + pageSize.value)
})

const cloneEntries = (entries: LocalEntry[]): LocalEntry[] => {
  return entries.map(e => ({ ...e }))
}

const loadEntries = async () => {
  if (!props.group) return
  const version = loadVersion
  const groupId = props.group.id
  loading.value = true
  // 切换到其他分组时丢弃旧草稿；同一分组重载（如保存部分失败）保留现有数据，
  // 任一接口失败时都不会把弹窗清空。
  if (loadedGroupId.value !== groupId) {
    serverEntries.value = []
    localEntries.value = []
  }
  try {
    const quotaRequest =
      typeof adminAPI.groups.getGroupQuotaMultipliers === 'function'
        ? adminAPI.groups.getGroupQuotaMultipliers(groupId)
        : Promise.resolve([] as GroupQuotaMultiplierEntry[])
    const [rateResult, quotaResult] = await Promise.allSettled([
      adminAPI.groups.getGroupRateMultipliers(groupId),
      quotaRequest
    ])
    if (version !== loadVersion) return

    const rateEntries = rateResult.status === 'fulfilled' ? rateResult.value : null
    const quotaEntries = quotaResult.status === 'fulfilled' ? quotaResult.value : null
    quotaLoadFailed.value = quotaEntries == null

    if (rateEntries == null && quotaEntries == null) {
      appStore.showError(t('admin.groups.failedToLoad'))
      console.error(
        'Error loading group rate multipliers:',
        rateResult.status === 'rejected' ? rateResult.reason : undefined,
        quotaResult.status === 'rejected' ? quotaResult.reason : undefined
      )
      return
    }
    if (rateEntries == null) {
      appStore.showError(t('admin.groups.failedToLoad'))
      console.error(
        'Error loading group rate multipliers:',
        rateResult.status === 'rejected' ? rateResult.reason : undefined
      )
    }
    if (quotaEntries == null) {
      appStore.showError(t('admin.groups.quotaMultiplierLoadFailed'))
      console.error(
        'Error loading group quota multipliers:',
        quotaResult.status === 'rejected' ? quotaResult.reason : undefined
      )
    }

    const merged = new Map<number, LocalEntry>()
    // 同组重载且额度接口失败时，保留上次成功加载的额度数据，避免空白/误清空。
    if (quotaEntries == null && loadedGroupId.value === groupId) {
      for (const entry of serverEntries.value) {
        merged.set(entry.user_id, { ...entry })
      }
    }
    for (const entry of quotaEntries ?? []) {
      merged.set(entry.user_id, {
        user_id: entry.user_id,
        user_name: entry.username || '',
        user_email: entry.email,
        user_notes: entry.notes || '',
        user_status: entry.status || 'active',
        rate_multiplier: entry.rate_multiplier ?? null,
        rpm_override: null,
        subscription_id: entry.subscription_id ?? null,
        quota_multiplier: entry.quota_multiplier ?? null,
        default_quota_multiplier: entry.default_quota_multiplier ?? null
      })
    }
    for (const entry of rateEntries ?? []) {
      const existing = merged.get(entry.user_id)
      if (existing) {
        existing.user_name = existing.user_name || entry.user_name || ''
        existing.user_email = existing.user_email || entry.user_email
        existing.user_notes = existing.user_notes || entry.user_notes || ''
        existing.user_status = existing.user_status || entry.user_status || 'active'
        existing.rate_multiplier = entry.rate_multiplier ?? existing.rate_multiplier ?? null
        existing.rpm_override = entry.rpm_override ?? existing.rpm_override ?? null
        continue
      }
      merged.set(entry.user_id, {
        ...entry,
        subscription_id: null,
        quota_multiplier: null,
        default_quota_multiplier: null
      })
    }
    // rate_multiplier 为空且没有任何额度倍率的条目只可能来自价格接口，保留展示；
    // 其余纯订阅用户同样保留。
    serverEntries.value = Array.from(merged.values())
    localEntries.value = cloneEntries(serverEntries.value)
    removedUserIds.value = []
    loadedGroupId.value = groupId
    adjustPage()
  } catch (error) {
    if (version !== loadVersion) return
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading group rate multipliers:', error)
  } finally {
    if (version === loadVersion) loading.value = false
  }
}

const adjustPage = () => {
  const totalPages = Math.max(1, Math.ceil(localEntries.value.length / pageSize.value))
  if (currentPage.value > totalPages) {
    currentPage.value = totalPages
  }
}

watch([() => props.show, () => props.group?.id], ([show]) => {
  loadVersion++
  if (show && props.group) {
    currentPage.value = 1
    batchFactor.value = null
    batchQuotaMultiplier.value = null
    showClearAllConfirm.value = false
    searchQuery.value = ''
    searchResults.value = []
    selectedUser.value = null
    newRate.value = null
    removedUserIds.value = []
    quotaLoadFailed.value = false
    loadEntries()
  }
})

const handlePageSizeChange = (newSize: number) => {
  pageSize.value = newSize
  currentPage.value = 1
}

const handleSearchUsers = () => {
  clearTimeout(searchTimeout)
  selectedUser.value = null
  if (!searchQuery.value.trim()) {
    searchResults.value = []
    showDropdown.value = false
    return
  }
  searchTimeout = setTimeout(async () => {
    try {
      const res = await adminAPI.users.list(1, 10, { search: searchQuery.value.trim() })
      searchResults.value = res.items
      showDropdown.value = true
    } catch {
      searchResults.value = []
    }
  }, 300)
}

const selectUser = (user: AdminUser) => {
  selectedUser.value = user
  searchQuery.value = user.email
  showDropdown.value = false
  searchResults.value = []
}

// 本地添加（或覆盖已有用户）
const handleAddLocal = () => {
  if (!selectedUser.value || newRate.value == null || newRate.value <= 0) return
  const user = selectedUser.value
  const idx = localEntries.value.findIndex(e => e.user_id === user.id)
  // 删除后又重新添加时，从服务端快照恢复额度字段，避免误清空原有额度设置。
  const existing =
    idx >= 0 ? localEntries.value[idx] : serverEntries.value.find(e => e.user_id === user.id)
  const entry: LocalEntry = {
    user_id: user.id,
    user_name: user.username || '',
    user_email: user.email,
    user_notes: user.notes || '',
    user_status: user.status || 'active',
    rate_multiplier: newRate.value,
    rpm_override: null,
    subscription_id: existing?.subscription_id ?? null,
    quota_multiplier: existing?.quota_multiplier ?? null,
    default_quota_multiplier: existing?.default_quota_multiplier ?? null
  }
  if (idx >= 0) {
    localEntries.value[idx] = entry
  } else {
    localEntries.value.push(entry)
  }
  // 用户重新被添加时撤销之前的额度清除标记，避免保存时又把它清掉。
  removedUserIds.value = removedUserIds.value.filter(id => id !== user.id)
  searchQuery.value = ''
  selectedUser.value = null
  newRate.value = null
  adjustPage()
}

// 本地修改倍率
const updateLocalRate = (userId: number, value: string) => {
  const entry = localEntries.value.find(e => e.user_id === userId)
  if (!entry) return
  if (value.trim() === '') {
    entry.rate_multiplier = null
    return
  }
  const num = parseFloat(value)
  if (!Number.isFinite(num) || num <= 0) return
  entry.rate_multiplier = num
}

// 本地修改额度倍率：留空表示清除（有订阅恢复 100%，无订阅删除默认值）；
// 非法值回填旧值并提示，避免静默提交旧数据。
const updateLocalQuotaMultiplier = (userId: number, value: string, input?: HTMLInputElement) => {
  if (quotaLoadFailed.value) return
  const entry = localEntries.value.find(e => e.user_id === userId)
  if (!entry) return
  // 有有效订阅时改订阅额度，否则改下次分配默认值，避免默认值被误标成“生效”。
  const hasActiveSubscription = entry.subscription_id != null
  const field = hasActiveSubscription ? 'quota_multiplier' : 'default_quota_multiplier'
  if (value.trim() === '') {
    if (hasActiveSubscription) {
      entry.quota_multiplier = 1
      if (input) input.value = '1'
    } else {
      entry.default_quota_multiplier = null
      if (input) input.value = ''
    }
    return
  }
  const numeric = parseQuotaMultiplier(value)
  if (numeric == null) {
    appStore.showError(t('admin.groups.quotaMultiplierRange'))
    if (input) {
      const current = quotaValue(entry)
      input.value = current == null ? '' : String(current)
    }
    return
  }
  entry[field] = numeric
  if (input) input.value = String(numeric)
}

// 本地删除价格倍率；若该用户同时有额度倍率/默认额度，记录 tombstone，
// 保存时额外提交 quota_multiplier: null，避免重开弹窗后额度配置“复活”。
const removeLocal = (userId: number) => {
  const entry = localEntries.value.find(e => e.user_id === userId)
  if (
    entry &&
    (entry.quota_multiplier != null || entry.default_quota_multiplier != null) &&
    !removedUserIds.value.includes(userId)
  ) {
    removedUserIds.value = [...removedUserIds.value, userId]
  }
  localEntries.value = localEntries.value.filter(e => e.user_id !== userId)
  adjustPage()
}

// 批量乘数应用到本地
const applyBatchFactor = () => {
  if (!batchFactor.value || batchFactor.value <= 0) return
  for (const entry of localEntries.value) {
    if (entry.rate_multiplier != null) {
      entry.rate_multiplier = parseFloat((entry.rate_multiplier * batchFactor.value).toFixed(6))
    }
  }
  batchFactor.value = null
}

// 批量设置额度倍率
const applyBatchQuotaMultiplier = () => {
  if (quotaLoadFailed.value) return
  const value = parseQuotaMultiplier(batchQuotaMultiplier.value)
  if (value == null) return
  for (const entry of localEntries.value) {
    if (entry.subscription_id != null) {
      entry.quota_multiplier = value
    } else {
      entry.default_quota_multiplier = value
    }
  }
  batchQuotaMultiplier.value = null
}

// 本地清空：先二次确认，确认后价格倍率与额度倍率一起提交清空。
const requestClearAllLocal = () => {
  showClearAllConfirm.value = true
}

const confirmClearAllLocal = () => {
  localEntries.value = []
  // 空数组本身即“清空整组额度”，无需再叠加逐用户 tombstone。
  removedUserIds.value = []
  batchQuotaMultiplier.value = null
  showClearAllConfirm.value = false
}

// 取消：恢复到服务器数据
const handleCancel = () => {
  localEntries.value = cloneEntries(serverEntries.value)
  removedUserIds.value = []
  batchFactor.value = null
  batchQuotaMultiplier.value = null
  adjustPage()
}

// 保存：价格倍率走原接口，额度倍率走新接口，不能混用。
// 两个接口独立结算：任一失败时保留成功的一侧，并重新拉取失败侧的服务端数据。
const handleSave = async () => {
  if (!props.group || saving.value) return
  saving.value = true
  try {
    const rateEntries = localEntries.value
      .filter(e => e.rate_multiplier != null)
      .map(e => ({
        user_id: e.user_id,
        rate_multiplier: e.rate_multiplier as number
      }))
    const quotaEntries = [
      ...localEntries.value.map(e => ({
        user_id: e.user_id,
        // 有订阅清空统一提交 1.0；无订阅清空提交 null 删除默认值。
        quota_multiplier:
          e.subscription_id == null ? (e.default_quota_multiplier ?? null) : (e.quota_multiplier ?? 1)
      })),
      // tombstone：删除行时显式提交 null，清除订阅额度/下次分配默认值。
      ...removedUserIds.value.map(userId => ({
        user_id: userId,
        quota_multiplier: null
      }))
    ]
    // 额度数据未成功加载时绝不提交额度（null 会误清空），只保存价格倍率。
    const quotaSupported =
      !quotaLoadFailed.value &&
      typeof adminAPI.groups.batchSetGroupQuotaMultipliers === 'function'
    const quotaSave = quotaSupported
      ? adminAPI.groups.batchSetGroupQuotaMultipliers(props.group.id, quotaEntries)
      : Promise.resolve({ message: '' })
    const [rateResult, quotaResult] = await Promise.allSettled([
      adminAPI.groups.batchSetGroupRateMultipliers(props.group.id, rateEntries),
      quotaSave
    ])
    const rateSaved = rateResult.status === 'fulfilled'
    const quotaSaved = !quotaSupported || quotaResult.status === 'fulfilled'
    if (rateSaved && quotaSaved) {
      appStore.showSuccess(
        quotaSupported ? t('admin.groups.rateSaved') : t('admin.groups.rateMultiplierSaved')
      )
      removedUserIds.value = []
      emit('success')
      emit('close')
      return
    }

    if (rateSaved) {
      appStore.showSuccess(t('admin.groups.rateMultiplierSaved'))
    } else {
      appStore.showError(t('admin.groups.rateMultiplierSaveFailed'))
      console.error('Error saving rate multipliers:', rateResult.reason)
    }
    if (quotaSupported) {
      if (quotaSaved) {
        appStore.showSuccess(t('admin.groups.quotaMultiplierSaved'))
      } else {
        appStore.showError(t('admin.groups.quotaMultiplierSaveFailed'))
        console.error('Error saving quota multipliers:', quotaResult.reason)
      }
    }
    // 重新同步服务端数据：成功的一侧已落库，失败的一侧回到旧值，避免“撤销修改”回退到错误快照。
    await loadEntries()
  } catch (error) {
    appStore.showError(t('admin.groups.failedToSave'))
    console.error('Error saving rate multipliers:', error)
  } finally {
    saving.value = false
  }
}

// 关闭时如果有未保存修改，先恢复
const handleClose = () => {
  showClearAllConfirm.value = false
  if (isDirty.value) {
    localEntries.value = cloneEntries(serverEntries.value)
  }
  removedUserIds.value = []
  emit('close')
}

// 点击外部关闭下拉
const handleClickOutside = () => {
  showDropdown.value = false
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', handleClickOutside)
}
onUnmounted(() => {
  loadVersion++
  clearTimeout(searchTimeout)
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.hide-spinner::-webkit-outer-spin-button,
.hide-spinner::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.hide-spinner {
  -moz-appearance: textfield;
}
</style>
