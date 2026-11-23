import { describe, expect, it } from 'vitest'

import {
  PROJECT_DOCKER_IMAGE,
  PROJECT_DOCS_URL,
  PROJECT_REPO,
  PROJECT_URL
} from '@/constants/project'

describe('project links', () => {
  it('points at the ReSub2API fork', () => {
    expect(PROJECT_REPO).toBe('FurWolf19/resub2api')
    expect(PROJECT_URL).toBe('https://github.com/FurWolf19/resub2api')
    expect(PROJECT_DOCS_URL).toBe('https://github.com/FurWolf19/resub2api/blob/main/docs')
  })

  it('uses the fork Docker image', () => {
    expect(PROJECT_DOCKER_IMAGE).toBe('njmlp/resub2api')
    expect(PROJECT_DOCKER_IMAGE).not.toContain('weishaw')
  })
})
