import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import OpenAIProtocolCapabilityCell from '../OpenAIProtocolCapabilityCell.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${Object.values(params).join(',')}` : key
    })
  }
})

const account = (overrides: Partial<Account> = {}): Account =>
  ({
    id: 1,
    name: 'OpenAI API Key',
    platform: 'openai',
    type: 'apikey',
    ...overrides
  }) as Account

const mountCell = (value: Account) =>
  mount(OpenAIProtocolCapabilityCell, { props: { account: value } })

const badge = (wrapper: ReturnType<typeof mountCell>, protocol: string) =>
  wrapper.get(`[data-protocol="${protocol}"]`)

describe('OpenAIProtocolCapabilityCell', () => {
  it('renders supported and unsupported states from the new capability key', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: true, probed_at: '2026-05-01T08:00:00Z' },
            chat_completions: { supported: false, probed_at: '2026-05-01T08:00:00Z' }
          }
        }
      })
    )

    expect(badge(wrapper, 'responses').attributes('data-status')).toBe('supported')
    expect(badge(wrapper, 'responses').classes()).toContain('bg-emerald-100')
    expect(badge(wrapper, 'chat_completions').attributes('data-status')).toBe('unsupported')
    expect(badge(wrapper, 'chat_completions').classes()).toContain('bg-red-100')
    expect(wrapper.find('[data-testid="protocol-badge-mode"]').exists()).toBe(false)
  })

  it('falls back to the legacy responses flag when the new key is absent', () => {
    const supported = mountCell(account({ extra: { openai_responses_supported: true } }))
    expect(badge(supported, 'responses').attributes('data-status')).toBe('supported')
    // 旧键存在时后端把 Chat Completions 视为支持，前端必须一致，不能显示未知。
    expect(badge(supported, 'chat_completions').attributes('data-status')).toBe('supported')

    const unsupported = mountCell(account({ extra: { openai_responses_supported: false } }))
    expect(badge(unsupported, 'responses').attributes('data-status')).toBe('unsupported')
    expect(badge(unsupported, 'chat_completions').attributes('data-status')).toBe('supported')
  })

  it('keeps the new key authoritative over the legacy flag', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: { responses: { supported: false } },
          openai_responses_supported: true
        }
      })
    )

    expect(badge(wrapper, 'responses').attributes('data-status')).toBe('unsupported')
    // 新键缺少 chat_completions，旧键仍按后端语义补成支持。
    expect(badge(wrapper, 'chat_completions').attributes('data-status')).toBe('supported')

    const complete = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: false },
            chat_completions: { supported: false }
          },
          openai_responses_supported: true
        }
      })
    )
    expect(badge(complete, 'responses').attributes('data-status')).toBe('unsupported')
    expect(badge(complete, 'chat_completions').attributes('data-status')).toBe('unsupported')
  })

  it('falls back to the legacy flag when a new-key entry is unparsable', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: 'yes' },
            chat_completions: { supported: null }
          },
          openai_responses_supported: false
        }
      })
    )

    expect(badge(wrapper, 'responses').attributes('data-status')).toBe('unsupported')
    expect(badge(wrapper, 'chat_completions').attributes('data-status')).toBe('supported')
  })

  it('renders a placeholder for non-APIKey OpenAI accounts', () => {
    for (const type of ['oauth', 'upstream', 'setup-token'] as const) {
      const wrapper = mountCell(
        account({ type, extra: { openai_responses_supported: true, openai_responses_mode: 'force_responses' } })
      )

      expect(wrapper.find('[data-testid="openai-protocol-capabilities"]').exists()).toBe(false)
      expect(wrapper.text()).toBe('-')
    }
  })

  it('exposes protocol name and status in aria-label', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: true },
            chat_completions: { supported: false }
          }
        }
      })
    )

    expect(badge(wrapper, 'responses').attributes('aria-label')).toBe(
      'admin.accounts.protocolCapabilities.responses · admin.accounts.protocolCapabilities.status.supported'
    )
    expect(badge(wrapper, 'chat_completions').attributes('aria-label')).toBe(
      'admin.accounts.protocolCapabilities.chatCompletions · admin.accounts.protocolCapabilities.status.unsupported'
    )
    expect(wrapper.find('[data-testid="protocol-badge-mode"]').exists()).toBe(false)
  })

  it('shows both protocols as unknown without probe data', () => {
    const wrapper = mountCell(account({ extra: {} }))

    for (const protocol of ['responses', 'chat_completions']) {
      expect(badge(wrapper, protocol).attributes('data-status')).toBe('unknown')
      expect(badge(wrapper, protocol).classes()).toContain('bg-gray-100')
    }
  })

  it('treats a non-boolean supported value as unknown', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: 'yes' },
            chat_completions: { supported: 1 }
          }
        }
      })
    )

    expect(badge(wrapper, 'responses').attributes('data-status')).toBe('unknown')
    expect(badge(wrapper, 'chat_completions').attributes('data-status')).toBe('unknown')
  })

  it('formats string and numeric probed_at values into the tooltip', () => {
    const iso = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: { responses: { supported: true, probed_at: '2026-05-01T08:00:00Z' } }
        }
      })
    )
    expect(badge(iso, 'responses').attributes('title')).toContain(
      'admin.accounts.protocolCapabilities.probedAt|'
    )
    expect(badge(iso, 'responses').attributes('title')).toContain('2026')

    const unixSeconds = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: { responses: { supported: true, probed_at: 1780000000 } }
        }
      })
    )
    expect(badge(unixSeconds, 'responses').attributes('title')).toContain(
      'admin.accounts.protocolCapabilities.probedAt|'
    )

    const invalid = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: { responses: { supported: true, probed_at: 'not-a-date' } }
        }
      })
    )
    expect(badge(invalid, 'responses').attributes('title')).not.toContain(
      'admin.accounts.protocolCapabilities.probedAt'
    )
  })

  it('shows exactly two status badges in auto mode', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: {
            responses: { supported: true },
            chat_completions: { supported: false }
          }
        }
      })
    )

    expect(wrapper.findAll('[data-testid^="protocol-badge-"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="protocol-badge-forced"]').exists()).toBe(false)
  })

  it('shows one purple forced Responses badge instead of status badges', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_responses_supported: false,
          openai_responses_mode: 'force_responses'
        }
      })
    )

    const forced = wrapper.get('[data-testid="protocol-badge-forced"]')
    expect(forced.attributes('data-protocol')).toBe('responses')
    expect(forced.attributes('data-status')).toBe('forced')
    expect(forced.text()).toBe('admin.accounts.protocolCapabilities.forcedResponses')
    expect(forced.classes()).toContain('bg-purple-100')
    expect(forced.classes()).toContain('text-purple-700')
    expect(forced.classes()).toContain('dark:bg-purple-900/30')
    expect(forced.classes()).toContain('dark:text-purple-400')
    expect(wrapper.find('[data-testid="protocol-badge-responses"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="protocol-badge-chat_completions"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="protocol-badge-mode"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('bg-emerald-100')
    expect(forced.attributes('title')).toContain(
      'admin.accounts.protocolCapabilities.forcedOverrideHint'
    )
  })

  it('shows one purple forced Chat badge instead of status badges', () => {
    const wrapper = mountCell(
      account({
        extra: {
          openai_protocol_capabilities: { responses: { supported: true } },
          openai_responses_mode: 'force_chat_completions'
        }
      })
    )

    const forced = wrapper.get('[data-testid="protocol-badge-forced"]')
    expect(forced.attributes('data-protocol')).toBe('chat_completions')
    expect(forced.attributes('data-status')).toBe('forced')
    expect(forced.text()).toBe('admin.accounts.protocolCapabilities.forcedChat')
    expect(forced.classes()).toContain('bg-purple-100')
    expect(forced.attributes('aria-label')).toBe(
      'admin.accounts.protocolCapabilities.force · admin.accounts.protocolCapabilities.forcedChat'
    )
    expect(wrapper.find('[data-testid="protocol-badge-responses"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="protocol-badge-chat_completions"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('bg-emerald-100')
  })

  it('renders a placeholder for non-OpenAI platforms even with a forced mode', () => {
    const wrapper = mountCell(
      account({
        platform: 'anthropic',
        extra: { openai_responses_supported: true, openai_responses_mode: 'force_responses' }
      })
    )

    expect(wrapper.find('[data-testid="openai-protocol-capabilities"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="protocol-badge-forced"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('-')
  })
})
