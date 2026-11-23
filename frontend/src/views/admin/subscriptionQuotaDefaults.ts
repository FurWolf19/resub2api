import type { GroupQuotaMultiplierEntry } from '@/api/admin/groups'

/**
 * 分配弹窗预填用的“无订阅默认额度倍率”。
 * 只有当前没有生效订阅（subscription_id == null）的用户才应参与预填。
 */
export function collectAssignableQuotaDefaults(
  entries: GroupQuotaMultiplierEntry[]
): Map<number, number> {
  const defaults = new Map<number, number>()
  for (const entry of entries) {
    if (entry.subscription_id == null && entry.default_quota_multiplier != null) {
      defaults.set(entry.user_id, entry.default_quota_multiplier)
    }
  }
  return defaults
}
