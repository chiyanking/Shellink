import { describe, expect, it } from 'vitest'
import * as api from '../src/api.js'
import type { ShellinkClient } from '../src/types.js'

describe('api wrappers', () => {
  it('forwards method names and params', async () => {
    const calls: Array<{ method: string; params: unknown }> = []
    const client: ShellinkClient = {
      request: async (method, params) => {
        calls.push({ method, params })
        return { ok: true, id: 'x' }
      },
      close: () => undefined,
    }
    await api.listProfiles(client)
    await api.listProfiles(client, 'prod')
    await api.getProfile(client, 'p1')
    await api.createProfile(client, { name: 'n', host: 'h' })
    await api.updateProfile(client, 'p1', { name: 'n2' })
    await api.deleteProfile(client, 'p1')
    await api.listSessions(client)
    await api.createSession(client, 'p1')
    await api.createSession(client, 'p1', 80, 24)
    await api.sessionState(client, 's1')
    await api.sessionHistory(client, 's1')
    await api.sessionHistory(client, 's1', 3, 10)
    await api.sessionInput(client, 's1', 'yes', false)
    await api.sessionExecStart(client, 's1', 'uname')
    await api.sessionExecStart(client, 's1', 'uname', 5000)
    await api.sessionMode(client, 's1', 'AUTO')
    await api.sessionClose(client, 's1')
    await api.sessionUploadStart(client, 's1', '/r', new Uint8Array([1]))
    await api.sessionUploadStart(client, 's1', '/r', new Uint8Array([1]), 1000)
    await api.sessionDownloadStart(client, 's1', '/r', '/tmp/out')
    await api.sessionDownloadStart(client, 's1', '/r', '/tmp/out', 1000)
    await api.sessionEditStart(client, 's1', '/r', [{ oldText: 'a', newText: 'b' }])
    await api.sessionEditStart(client, 's1', '/r', [{ oldText: 'a', newText: 'b' }], 1000)
    expect(calls.map(call => call.method)).toEqual([
      'profiles.list',
      'profiles.list',
      'profiles.get',
      'profiles.create',
      'profiles.update',
      'profiles.delete',
      'sessions.list',
      'sessions.create',
      'sessions.create',
      'sessions.state',
      'sessions.history',
      'sessions.history',
      'sessions.input',
      'sessions.execStart',
      'sessions.execStart',
      'sessions.mode',
      'sessions.close',
      'sessions.uploadStart',
      'sessions.uploadStart',
      'sessions.downloadStart',
      'sessions.downloadStart',
      'sessions.editStart',
      'sessions.editStart',
    ])
    expect(calls[0]?.params).toEqual({})
    expect(calls[1]?.params).toEqual({ q: 'prod' })
    expect(calls.find(call => call.method === 'sessions.create' && (call.params as { cols?: number }).cols === 80)?.params)
      .toEqual({ profileId: 'p1', cols: 80, rows: 24 })
  })
})
