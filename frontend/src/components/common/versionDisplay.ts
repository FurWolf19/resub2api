import type { UpstreamInfo } from '@/api/admin/system'

export type VersionBadgeKind =
  | 'own-update'
  | 'upstream-update'
  | 'fork-release-missing'
  | 'up-to-date'

export interface VersionDisplayInput {
  hasUpdate: boolean
  ownReleaseAvailable?: boolean
  upstream?: UpstreamInfo | null
}

export interface VersionDisplay {
  badgeKind: VersionBadgeKind
  badgeTitleKey: string
  showOwnUpdate: boolean
  showForkMissing: boolean
  showUpToDateCheck: boolean
  showUpstreamNotice: boolean
  upstreamHasUpdate: boolean
}

/**
 * Resolves the version badge state. `own_release_available` is optional so a
 * new frontend talking to an older backend keeps the previous behavior instead
 * of claiming the fork has no release.
 */
export function resolveVersionDisplay(input: VersionDisplayInput): VersionDisplay {
  const ownReleaseAvailable = input.ownReleaseAvailable !== false
  const upstreamHasUpdate = input.upstream?.has_update === true
  const showUpstreamNotice = Boolean(input.upstream?.latest_version)

  if (input.hasUpdate) {
    return {
      badgeKind: 'own-update',
      badgeTitleKey: 'version.updateAvailable',
      showOwnUpdate: true,
      showForkMissing: false,
      showUpToDateCheck: false,
      showUpstreamNotice,
      upstreamHasUpdate
    }
  }

  if (!ownReleaseAvailable) {
    return {
      badgeKind: 'fork-release-missing',
      badgeTitleKey: 'version.forkReleaseMissing',
      showOwnUpdate: false,
      showForkMissing: true,
      showUpToDateCheck: false,
      showUpstreamNotice,
      upstreamHasUpdate
    }
  }

  if (upstreamHasUpdate) {
    return {
      badgeKind: 'upstream-update',
      badgeTitleKey: 'version.upstreamAvailable',
      showOwnUpdate: false,
      showForkMissing: false,
      showUpToDateCheck: false,
      showUpstreamNotice: true,
      upstreamHasUpdate: true
    }
  }

  return {
    badgeKind: 'up-to-date',
    badgeTitleKey: 'version.upToDate',
    showOwnUpdate: false,
    showForkMissing: false,
    showUpToDateCheck: true,
    showUpstreamNotice,
    upstreamHasUpdate: false
  }
}
