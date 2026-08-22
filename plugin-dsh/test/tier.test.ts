import { describe, expect, it } from 'vitest'
import { compileApproval, compileUserRegex, Config } from '../src/config.js'
import { isProductionTarget, SessionTargetCache } from '../src/tier.js'

describe('isProductionTarget', () => {
  const approval = Config().approval
  const compiled = compileApproval(approval)

  it('matches default (?i)prod / prd patterns on name host target uniqueId', () => {
    expect(isProductionTarget({ name: 'prod-web-01' }, approval, compiled)).toBe(true)
    expect(isProductionTarget({ host: 'prd.example.com' }, approval, compiled)).toBe(true)
    expect(isProductionTarget({ target: 'user@PROD-db' }, approval, compiled)).toBe(true)
    expect(isProductionTarget({ uniqueId: 'guid-prod' }, approval, compiled)).toBe(true)
    expect(isProductionTarget({ name: 'staging-web' }, approval, compiled)).toBe(false)
  })

  it('matches configured profile ids', () => {
    const cfg = { ...approval, productionProfileIds: ['p-prod'] }
    expect(isProductionTarget({ id: 'p-prod', name: 'web' }, cfg, compiled)).toBe(true)
    expect(isProductionTarget({ id: 'p-dev', name: 'web' }, cfg, compiled)).toBe(false)
  })

  it('treats everything as production when defaultTier is production', () => {
    const cfg = { ...approval, defaultTier: 'production' as const }
    expect(isProductionTarget({ name: 'dev-box' }, cfg, compiled)).toBe(true)
  })
})

describe('compileUserRegex / compileApproval', () => {
  it('compiles a plain pattern and a (?i) prefix', () => {
    expect(compileUserRegex('prod', 't').test('prod')).toBe(true)
    expect(compileUserRegex('prod', 't').test('PROD')).toBe(false)
    expect(compileUserRegex('(?i)prod', 't').test('PROD')).toBe(true)
  })

  it('throws on an invalid extraDanger pattern at compile time', () => {
    const approval = { ...Config().approval, extraDangerPatterns: ['('] }
    expect(() => compileApproval(approval)).toThrow(/invalid approval regex extraDangerPatterns\[0\]/)
  })

  it('compiles allow and production pattern lists', () => {
    const compiled = compileApproval({
      ...Config().approval,
      allowPatterns: ['^ls '],
      extraDangerPatterns: ['dangerous'],
    })
    expect(compiled.allow[0]?.test('ls /tmp')).toBe(true)
    expect(compiled.extraDanger[0]?.test('dangerous rm')).toBe(true)
  })
})

describe('SessionTargetCache', () => {
  it('stores, ingests, and clears rows', () => {
    const cache = new SessionTargetCache()
    cache.set('s1', { profileId: 'p1', profileName: 'web', target: '10.0.0.1' })
    expect(cache.get('s1')?.target).toBe('10.0.0.1')
    cache.ingest([{ id: 's2', profileId: 'p2', profileName: 'db', target: '10.0.0.2' }])
    expect(cache.get('s2')?.profileName).toBe('db')
    cache.delete('s1')
    expect(cache.get('s1')).toBeUndefined()
    cache.clear()
    expect(cache.get('s2')).toBeUndefined()
  })
})
