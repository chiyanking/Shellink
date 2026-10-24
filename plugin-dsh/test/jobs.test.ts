import { describe, expect, it } from 'vitest'
import { pollDaemonJob, waitForDaemonJob } from '../src/jobs.js'
import type { DaemonJobSnapshot, ExecStatusResult, ShellinkClient } from '../src/types.js'

function job(status: DaemonJobSnapshot['status'], id = 'job1'): DaemonJobSnapshot {
  return {
    id,
    sessionId: 's1',
    kind: 'exec',
    command: 'uname',
    remotePath: null,
    startSeq: 1,
    startedAt: 0,
    endedAt: status === 'RUNNING' || status === 'TIMED_OUT' ? null : 1,
    timeoutMs: 20_000,
    status,
    state: 'WAITING_INPUT',
    error: status === 'FAILED' ? 'boom' : null,
    result: null,
  }
}

function mockStatusClient(replies: ExecStatusResult[]): ShellinkClient {
  let index = 0
  return {
    request: async (method) => {
      if (method === 'sessions.execCancel') return job('CANCELED')
      if (method !== 'sessions.execStatus') throw new Error(method)
      const current = replies[Math.min(index, replies.length - 1)]!
      if (index < replies.length - 1) index++
      return current
    },
    subscribe: async () => {
      throw new Error('unused')
    },
    close: () => undefined,
  }
}

describe('pollDaemonJob', () => {
  it('does not settle on TIMED_OUT and continues until DONE', async () => {
    const client = mockStatusClient([
      { job: job('TIMED_OUT'), output: 'partial\n', cursor: 2, done: false },
      { job: job('DONE'), output: 'rest\n', cursor: 3, done: true },
    ])
    const hooks = pollDaemonJob(client, 'job1', 1, 1)
    const result = await hooks.done
    expect(result.status).toBe('completed')
    expect(result.output).toBe('partial\nrest\n')
  })

  it('maps CANCELED to killed and FAILED to failed', async () => {
    const canceled = pollDaemonJob(mockStatusClient([
      { job: job('CANCELED'), output: '', cursor: 1, done: true },
    ]), 'job1', 1, 1)
    expect((await canceled.done).status).toBe('killed')

    const failed = pollDaemonJob(mockStatusClient([
      { job: job('FAILED'), output: '', cursor: 1, done: true },
    ]), 'job1', 1, 1)
    expect(await failed.done).toMatchObject({ status: 'failed', detail: 'boom' })
  })

  it('cancel stops polling and requests execCancel', async () => {
    let canceled = false
    const client: ShellinkClient = {
      request: async (method) => {
        if (method === 'sessions.execCancel') {
          canceled = true
          return job('CANCELED')
        }
        return { job: job('RUNNING'), output: '', cursor: 1, done: false }
      },
      subscribe: async () => { throw new Error('unused') },
      close: () => undefined,
    }
    const hooks = pollDaemonJob(client, 'job1', 1, 5)
    hooks.cancel()
    expect((await hooks.done).status).toBe('killed')
    expect(canceled).toBe(true)
  })
})

describe('waitForDaemonJob', () => {
  it('returns the settled snapshot', async () => {
    const client = mockStatusClient([
      { job: job('DONE'), output: 'ok', cursor: 2, done: true },
    ])
    const result = await waitForDaemonJob(client, 'job1', 1)
    expect(result.job.status).toBe('DONE')
    expect(result.output).toContain('ok')
  })

  it('falls back to the last status output when the buffer is empty', async () => {
    const client = mockStatusClient([
      { job: job('DONE'), output: '', cursor: 2, done: true },
      { job: job('DONE'), output: 'from-last', cursor: 2, done: true },
    ])
    const result = await waitForDaemonJob(client, 'job1', 1)
    expect(result.output).toBe('from-last')
  })

  it('uses the job status as the failed detail when error is missing', async () => {
    const failed = pollDaemonJob(mockStatusClient([
      { job: { ...job('FAILED'), error: null }, output: '', cursor: 1, done: true },
    ]), 'job1', 1, 1)
    expect(await failed.done).toMatchObject({ status: 'failed', detail: 'FAILED' })
  })

  it('cancels when the abort signal fires', async () => {
    const client = mockStatusClient([
      { job: job('RUNNING'), output: 'partial', cursor: 1, done: false },
    ])
    const ac = new AbortController()
    const pending = waitForDaemonJob(client, 'job1', 1, ac.signal)
    ac.abort()
    const result = await pending
    expect(result.job.status).toBe('RUNNING')
  })
})
