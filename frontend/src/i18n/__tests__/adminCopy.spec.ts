import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('admin quota default copy', () => {
  it('describes the quota default as prefill-only in Chinese', () => {
    const copy = zh.admin.groups.quotaMultiplierDefaultHint

    expect(copy).toContain('仅用于分配弹窗预填')
    expect(copy).toContain('不会自动应用')
    expect(copy).not.toContain('保存后作为下次分配')
  })

  it('describes the quota default as prefill-only in English', () => {
    const copy = en.admin.groups.quotaMultiplierDefaultHint

    expect(copy).toContain('prefills the assignment dialog')
    expect(copy).toContain('not applied automatically')
    expect(copy).not.toContain('saved as the default for the next assignment')
  })
})

describe('admin five-hour reset copy', () => {
  it('explains the five-hour reset as next-request based in Chinese', () => {
    const copy = zh.admin.subscriptions.bulk.resetHint

    expect(copy).toContain('5 小时窗口会被清除')
    expect(copy).toContain('下一笔消费重新计时')
    expect(copy).not.toContain('从今天开始重新计算')
  })

  it('explains the five-hour reset as next-request based in English', () => {
    const copy = en.admin.subscriptions.bulk.resetHint

    expect(copy).toContain('5-hour window is cleared')
    expect(copy).toContain('next billable request')
    expect(copy).not.toContain('restarted from today')
  })

  it('keeps the reset confirmation consistent with the reset hint', () => {
    expect(zh.admin.subscriptions.resetQuotaConfirm).toContain('下一笔消费时重新计时')
    expect(zh.admin.subscriptions.resetQuotaConfirm).not.toContain('5 小时窗口会立即重新计算')
    expect(en.admin.subscriptions.resetQuotaConfirm).toContain('next billable request')
    expect(en.admin.subscriptions.resetQuotaConfirm).not.toContain(
      '5-hour window restarts immediately'
    )
  })
})
