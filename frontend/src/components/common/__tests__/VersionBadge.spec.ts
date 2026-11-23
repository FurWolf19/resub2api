import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'

import VersionBadge from '@/components/common/VersionBadge.vue'

const appState = {
  versionLoading: false,
  currentVersion: '0.2.24',
  latestVersion: '0.2.24',
  hasUpdate: false,
  releaseInfo: null,
  buildType: 'release',
  ownReleaseAvailable: true,
  upstream: null as Record<string, unknown> | null,
  fetchVersion: vi.fn(),
  clearVersionCache: vi.fn()
}

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => appState
}))

vi.mock('@/api/admin/system', () => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: ref(false), copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params && 'version' in params ? `${key}:${params.version}` : key
  })
}))

async function openBadge() {
  const wrapper = mount(VersionBadge, {
    props: { version: '0.2.24' },
    global: { stubs: { Icon: true } }
  })
  await wrapper.find('button').trigger('click')
  await nextTick()
  return wrapper
}

describe('VersionBadge upstream notice', () => {
  beforeEach(() => {
    appState.hasUpdate = false
    appState.ownReleaseAvailable = true
    appState.upstream = null
    appState.fetchVersion.mockClear()
  })

  it('shows the upstream latest version without claiming up to date', async () => {
    appState.upstream = {
      repository: 'Wei-Shaw/sub2api',
      latest_version: '0.2.25',
      has_update: true,
      html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25'
    }

    const wrapper = await openBadge()
    const text = wrapper.text()

    expect(text).toContain('v0.2.25')
    expect(text).toContain('version.upstreamRelease')
    expect(text).not.toContain('version.upToDate')
  })

  it('shows the missing fork release state instead of a green up-to-date state', async () => {
    appState.ownReleaseAvailable = false

    const wrapper = await openBadge()
    const text = wrapper.text()

    expect(text).toContain('version.forkReleaseMissing')
    expect(text).toContain('version.forkReleaseMissingHint')
    expect(text).not.toContain('version.upToDate')
  })
})
