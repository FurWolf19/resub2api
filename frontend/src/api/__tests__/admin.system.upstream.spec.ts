import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn()
}))

vi.mock('../client', () => ({
  apiClient: {
    get
  }
}))

import { checkUpdates } from '@/api/admin/system'

describe('admin system upstream API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('passes through the fork repository and upstream notice', async () => {
    const payload = {
      current_version: '0.2.24',
      latest_version: '0.2.24',
      has_update: false,
      cached: false,
      build_type: 'release',
      repository: 'FurWolf19/resub2api',
      own_release_available: true,
      upstream: {
        repository: 'Wei-Shaw/sub2api',
        latest_tag: 'v0.2.25',
        latest_version: '0.2.25',
        has_update: true,
        html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25',
        checked_at: '2026-11-24T00:00:00Z'
      }
    }
    get.mockResolvedValue({ data: payload })

    const result = await checkUpdates()

    expect(get).toHaveBeenCalledWith('/admin/system/check-updates', { params: undefined })
    expect(result).toEqual(payload)
  })

  it('keeps the force query parameter unchanged', async () => {
    get.mockResolvedValue({
      data: {
        current_version: '0.2.24',
        latest_version: '0.2.24',
        has_update: false,
        cached: false,
        build_type: 'source'
      }
    })

    await checkUpdates(true)

    expect(get).toHaveBeenCalledWith('/admin/system/check-updates', { params: { force: 'true' } })
  })
})
