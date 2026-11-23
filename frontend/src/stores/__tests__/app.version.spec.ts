import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { checkUpdates } from '@/api/admin/system'
import { useAppStore } from '@/stores/app'

vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn()
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn()
}))

describe('app store version state', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.mocked(checkUpdates).mockReset()
  })

  it('maps fork release state and upstream notice from check-updates', async () => {
    vi.mocked(checkUpdates).mockResolvedValue({
      current_version: '0.2.24',
      latest_version: '0.2.24',
      has_update: false,
      cached: false,
      build_type: 'release',
      repository: 'FurWolf19/resub2api',
      own_release_available: false,
      upstream: {
        repository: 'Wei-Shaw/sub2api',
        latest_tag: 'v0.2.25',
        latest_version: '0.2.25',
        has_update: true,
        html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25',
        checked_at: '2026-11-24T00:00:00Z'
      }
    })

    const store = useAppStore()
    await store.fetchVersion()

    expect(store.repository).toBe('FurWolf19/resub2api')
    expect(store.ownReleaseAvailable).toBe(false)
    expect(store.upstream?.latest_version).toBe('0.2.25')
    expect(store.upstream?.has_update).toBe(true)
  })

  it('keeps a release available when an older backend omits the field', async () => {
    vi.mocked(checkUpdates).mockResolvedValue({
      current_version: '0.2.24',
      latest_version: '0.2.24',
      has_update: false,
      cached: false,
      build_type: 'source'
    })

    const store = useAppStore()
    await store.fetchVersion()

    expect(store.ownReleaseAvailable).toBe(true)
    expect(store.upstream).toBeNull()
  })

  it('returns cached state without another request and refetches when forced', async () => {
    vi.mocked(checkUpdates).mockResolvedValue({
      current_version: '0.2.24',
      latest_version: '0.2.24',
      has_update: false,
      cached: false,
      build_type: 'source',
      own_release_available: true
    })

    const store = useAppStore()
    await store.fetchVersion()
    const cached = await store.fetchVersion()

    expect(cached?.cached).toBe(true)
    expect(checkUpdates).toHaveBeenCalledTimes(1)

    await store.fetchVersion(true)
    expect(checkUpdates).toHaveBeenCalledTimes(2)
    expect(checkUpdates).toHaveBeenLastCalledWith(true)
  })
})
