import { describe, expect, it } from 'vitest'

import { resolveVersionDisplay } from '@/components/common/versionDisplay'

describe('resolveVersionDisplay', () => {
  it('prioritizes the fork own update', () => {
    const display = resolveVersionDisplay({
      hasUpdate: true,
      ownReleaseAvailable: true,
      upstream: { repository: 'Wei-Shaw/sub2api', latest_version: '0.2.25', has_update: true }
    })

    expect(display.badgeKind).toBe('own-update')
    expect(display.badgeTitleKey).toBe('version.updateAvailable')
    expect(display.showUpstreamNotice).toBe(true)
    expect(display.showUpToDateCheck).toBe(false)
  })

  it('shows an upstream notice when only upstream has a newer release', () => {
    const display = resolveVersionDisplay({
      hasUpdate: false,
      ownReleaseAvailable: true,
      upstream: { repository: 'Wei-Shaw/sub2api', latest_version: '0.2.25', has_update: true }
    })

    expect(display.badgeKind).toBe('upstream-update')
    expect(display.badgeTitleKey).toBe('version.upstreamAvailable')
    expect(display.showUpstreamNotice).toBe(true)
    expect(display.showUpToDateCheck).toBe(false)
  })

  it('reports a missing fork release instead of up to date', () => {
    const display = resolveVersionDisplay({
      hasUpdate: false,
      ownReleaseAvailable: false,
      upstream: { repository: 'Wei-Shaw/sub2api', latest_version: '0.2.24', has_update: false }
    })

    expect(display.badgeKind).toBe('fork-release-missing')
    expect(display.badgeTitleKey).toBe('version.forkReleaseMissing')
    expect(display.showForkMissing).toBe(true)
    expect(display.showUpToDateCheck).toBe(false)
  })

  it('keeps the missing-release state when upstream also has a newer release', () => {
    const display = resolveVersionDisplay({
      hasUpdate: false,
      ownReleaseAvailable: false,
      upstream: { repository: 'Wei-Shaw/sub2api', latest_version: '0.2.25', has_update: true }
    })

    expect(display.badgeKind).toBe('fork-release-missing')
    expect(display.badgeTitleKey).toBe('version.forkReleaseMissing')
    expect(display.showUpstreamNotice).toBe(true)
    expect(display.showUpToDateCheck).toBe(false)
  })

  it('treats an omitted own_release_available as available for old backends', () => {
    const display = resolveVersionDisplay({ hasUpdate: false, upstream: null })

    expect(display.badgeKind).toBe('up-to-date')
    expect(display.showUpToDateCheck).toBe(true)
  })
})
