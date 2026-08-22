import { describe, expect, it } from 'vitest'
import { Config } from '../src/config.js'

describe('Config schema', () => {
  it('fills defaults', () => {
    const value = Config()
    expect(value.autoStartDaemon).toBe(true)
    expect(value.profileWrites).toBe(true)
    expect(value.uploadMaxBytes).toBe(6_291_456)
    expect(value.execDefaultTimeoutMs).toBe(20_000)
    expect(value.approval.mode).toBe('enforce')
    expect(value.approval.connect).toBe('always')
    expect(value.approval.remember).toBe('connect+command')
    expect(value.approval.productionPatterns).toEqual(['(?i)prod', '(?i)prd'])
    expect(value.approval.gateProfileDelete).toBe(false)
    expect(value.approval.gateSessionClose).toBe(false)
  })
})
