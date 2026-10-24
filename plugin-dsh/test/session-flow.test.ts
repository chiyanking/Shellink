import { describe, expect, it } from 'vitest'
import * as api from '../src/api.js'
import { awaitWaitingInput } from '../src/ready.js'
import { waitForDaemonJob } from '../src/jobs.js'
import type { ShellinkClient, SessionStateResult } from '../src/types.js'

/** In-memory daemon stand-in covering open → exec → close. */
function fakeSessionDaemon(): ShellinkClient {
  let state: SessionStateResult = { id: 's1', state: 'CONNECTING', mode: 'AUTO', active: true }
  let seq = 1
  let stateReads = 0
  return {
    request: async (method, params) => {
      const body = (params ?? {}) as Record<string, unknown>
      if (method === 'sessions.create') {
        return { id: 's1', state: state.state, mode: 'AUTO', target: 'local' }
      }
      if (method === 'sessions.state') {
        stateReads += 1
        if (stateReads > 1) state = { ...state, state: 'WAITING_INPUT' }
        return state
      }
      if (method === 'sessions.execStart') {
        return { jobId: 'job1', status: 'RUNNING', startSeq: seq, state: 'OUTPUTTING' }
      }
      if (method === 'sessions.execStatus') {
        seq += 1
        return {
          job: {
            id: 'job1',
            sessionId: 's1',
            kind: 'exec',
            command: 'uname -a',
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
          cursor: seq,
          done: true,
        }
      }
      if (method === 'sessions.close') {
        state = { ...state, state: 'DISCONNECTED', active: false }
        return { ok: true }
      }
      throw new Error(`unexpected ${method} ${JSON.stringify(body)}`)
    },
    close: () => undefined,
  }
}

describe('open → exec → close flow', () => {
  it('waits for WAITING_INPUT, runs a job, then closes', async () => {
    const client = fakeSessionDaemon()
    const created = await api.createSession(client, 'profile-1')
    const ready = await awaitWaitingInput({ client, sessionId: created.id, timeoutMs: 1000, pollMs: 5 })
    expect(ready.state).toBe('WAITING_INPUT')
    const start = await api.sessionExecStart(client, created.id, 'uname -a')
    const result = await waitForDaemonJob(client, start.jobId, start.startSeq)
    expect(result.job.status).toBe('DONE')
    expect(result.output).toContain('Linux')
    const closed = await api.sessionClose(client, created.id)
    expect(closed.ok).toBe(true)
  })
})
