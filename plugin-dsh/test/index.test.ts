import { afterEach, describe, expect, it, vi } from 'vitest'
import { Context } from '@deepseek-ai/cordis'
import { Config } from '../src/config.js'
import { MissingCredentialError } from '../src/errors.js'
import type { PublicProfile, ShellinkClient, SessionStateResult } from '../src/types.js'

const ensureDaemon = vi.hoisted(() => vi.fn())
const resolveBaseUrl = vi.hoisted(() => vi.fn((value?: string) => value ?? 'http://127.0.0.1:7070'))

vi.mock('../src/daemon.js', () => ({
  ensureDaemon,
  resolveBaseUrl,
}))

import ShellinkService, { formatShellinkError, inject, name } from '../src/index.js'

function profile(id = 'p1'): PublicProfile {
  return {
    id,
    name: 'dev',
    uniqueId: null,
    connectType: 'ssh',
    command: null,
    host: '127.0.0.1',
    port: 22,
    username: 'root',
    authType: 'key',
    hasPassword: false,
    hasPrivateKey: true,
    hasPassphrase: false,
    term: 'xterm',
    cols: 80,
    rows: 24,
    promptRegex: null,
    createdAt: 0,
    updatedAt: 0,
  }
}

function waiting(id = 's1'): SessionStateResult {
  return { id, state: 'WAITING_INPUT', mode: 'AUTO', active: true }
}

function fakeClient(): ShellinkClient {
  return {
    request: async (method, params) => {
      if (method === 'profiles.list') return [profile()]
      if (method === 'profiles.get') return profile((params as { id: string }).id)
      if (method === 'profiles.create' || method === 'profiles.update') return profile()
      if (method === 'profiles.delete') return { ok: true }
      if (method === 'sessions.list') return []
      if (method === 'sessions.create') {
        return { id: 's1', state: 'WAITING_INPUT', mode: 'AUTO', target: '127.0.0.1' }
      }
      if (method === 'sessions.state') return waiting()
      if (method === 'sessions.history') return { cursor: 1, text: 'hi' }
      if (method === 'sessions.input') return { ok: true, state: 'WAITING_INPUT', lastSeq: 1 }
      if (method === 'sessions.execStart') {
        return { jobId: 'job1', status: 'RUNNING', startSeq: 1, state: 'OUTPUTTING' }
      }
      if (method === 'sessions.execStatus') {
        return {
          job: {
            id: 'job1',
            sessionId: 's1',
            kind: 'exec',
            command: 'uname',
            remotePath: null,
            startSeq: 1,
            startedAt: 0,
            endedAt: 1,
            timeoutMs: 20_000,
            status: 'DONE',
            state: 'WAITING_INPUT',
            error: null,
            result: null,
          },
          output: 'Linux\n',
          cursor: 2,
          done: true,
        }
      }
      if (method === 'sessions.mode') return { ok: true, mode: (params as { mode: string }).mode }
      if (method === 'sessions.close') return { ok: true }
      if (method === 'sessions.uploadStart' || method === 'sessions.downloadStart' || method === 'sessions.editStart') {
        return {
          id: 'xfer1',
          sessionId: 's1',
          kind: method.includes('upload') ? 'upload' : method.includes('download') ? 'download' : 'edit',
          command: null,
          remotePath: '/tmp/x',
          startSeq: 1,
          startedAt: 0,
          endedAt: null,
          timeoutMs: 20_000,
          status: 'RUNNING',
          state: 'OUTPUTTING',
          error: null,
          result: null,
        }
    }
      throw new Error(method)
    },
    close: () => undefined,
  }
}

function makeService(secrets: Record<string, string> = { PW: 'secret' }): { ctx: Context; service: ShellinkService } {
  const ctx = new Context()
  ctx.provide('credentials', {
    resolve: async (ref: string) => {
      const value = secrets[ref]
      return value === undefined ? undefined : { value }
    },
  })
  const service = new ShellinkService(ctx, Config({ baseUrl: 'http://127.0.0.1:7070' }))
  return { ctx, service }
}

afterEach(() => {
  ensureDaemon.mockReset()
  resolveBaseUrl.mockClear()
})

describe('ShellinkService', () => {
  it('exports the plugin surface', () => {
    expect(name).toBe('shellink')
    expect(inject).toEqual(['credentials'])
    expect(formatShellinkError(new Error('x'))).toBe('x')
  })

  it('reuses one daemon HTTP client', async () => {
    const client = fakeClient()
    ensureDaemon.mockResolvedValue(client)
    const { service } = makeService()
    const first = service.ensureClient()
    const second = service.ensureClient()
    expect(await first).toBe(client)
    expect(await second).toBe(client)
    expect(ensureDaemon).toHaveBeenCalledTimes(1)
    expect(await service.ensureClient()).toBe(client)
    expect(resolveBaseUrl).toHaveBeenCalledWith('http://127.0.0.1:7070')
  })

  it('clears a failed connect so the next call retries', async () => {
    ensureDaemon.mockRejectedValueOnce(new Error('down'))
    const { service } = makeService()
    await expect(service.ensureClient()).rejects.toThrow(/down/)
    const client = fakeClient()
    ensureDaemon.mockResolvedValue(client)
    expect(await service.ensureClient()).toBe(client)
  })

  it('covers profile, session, job, and transfer methods', async () => {
    ensureDaemon.mockResolvedValue(fakeClient())
    const { service } = makeService()
    expect(await service.listProfiles('prod')).toEqual([profile()])
    expect(await service.getProfile('p1')).toEqual(profile('p1'))
    expect(await service.createProfile({ name: 'n', passwordRef: 'PW' })).toEqual(profile())
    expect(await service.updateProfile('p1', { host: 'h' })).toEqual(profile())
    expect(await service.deleteProfile('p1')).toEqual({ ok: true })
    expect(await service.listSessions()).toEqual([])
    const opened = await service.openSession('p1', 80, 24)
    expect(opened.id).toBe('s1')
    expect(service.sessionTargets.get('s1')?.target).toBe('127.0.0.1')
    expect(await service.sessionState('s1')).toEqual(waiting())
    expect(await service.sessionHistory('s1', 0, 10)).toEqual({ cursor: 1, text: 'hi' })
    expect(await service.sessionInput('s1', 'y')).toMatchObject({ ok: true })
    expect(await service.execStart('s1', 'uname')).toMatchObject({ jobId: 'job1' })
    const fg = await service.execForeground('s1', 'uname')
    expect(fg).toMatchObject({ jobId: 'job1', status: 'DONE', output: 'Linux\n' })
    expect(await service.uploadStart('s1', '/tmp/a', new Uint8Array([1]))).toMatchObject({ kind: 'upload' })
    expect(await service.downloadStart('s1', '/tmp/a', '/tmp/b')).toMatchObject({ kind: 'download' })
    expect(await service.editStart('s1', '/tmp/a', [{ oldText: 'a', newText: 'b' }])).toMatchObject({ kind: 'edit' })
    expect(await service.sessionMode('s1', 'MANUAL')).toEqual({ ok: true, mode: 'MANUAL' })
    expect(await service.sessionClose('s1')).toEqual({ ok: true })
    expect(service.sessionTargets.get('s1')).toBeUndefined()
  })

  it('polls jobs through the connected HTTP client', async () => {
    const client = fakeClient()
    ensureDaemon.mockResolvedValue(client)
    const { service } = makeService()
    expect(() => service.pollJob('job1', 1)).toThrow(/not connected/)
    await service.ensureClient()
    const hooks = service.pollJob('job1', 1)
    expect((await hooks.done).status).toBe('completed')
  })

  it('resolves credential refs and fails loud on a missing ref', async () => {
    ensureDaemon.mockResolvedValue(fakeClient())
    const { service } = makeService({})
    await expect(service.createProfile({ name: 'n', passwordRef: 'PW' }))
      .rejects.toBeInstanceOf(MissingCredentialError)
  })

  it('closes the HTTP client when the context effect tears down', async () => {
    const client = fakeClient()
    const close = vi.fn()
    client.close = close
    ensureDaemon.mockResolvedValue(client)
    const { ctx, service } = makeService()
    await service.ensureClient()
    service.sessionTargets.set('s1', { profileId: 'p1', profileName: 'n', target: 'h' })
    await ctx.fiber.dispose()
    expect(close).toHaveBeenCalled()
    expect(service.sessionTargets.get('s1')).toBeUndefined()
  })
})
