import { describe, expect, it } from 'vitest'
import type { RpcEvent } from '@shellink/protocol'
import { SessionNotReadyError } from '../src/errors.js'
import { awaitWaitingInput } from '../src/ready.js'
import type { RpcClient, SessionStateResult } from '../src/types.js'

function mockClient(states: SessionStateResult[]): RpcClient {
  let index = 0
  const listeners: Array<(event: RpcEvent) => void> = []
  return {
    request: async (method) => {
      if (method !== 'sessions.state') throw new Error(method)
      const current = states[Math.min(index, states.length - 1)]!
      if (index < states.length - 1) index++
      return current
    },
    subscribe: async (_params, listener) => {
      listeners.push(listener)
      return {
        subscriptionId: 'sub',
        initial: {},
        detach: () => undefined,
        unsubscribe: async () => undefined,
      }
    },
    close: () => undefined,
  }
}

const waiting: SessionStateResult = { id: 's1', state: 'WAITING_INPUT', mode: 'AUTO', active: true }
const connecting: SessionStateResult = { id: 's1', state: 'CONNECTING', mode: 'AUTO', active: true }
const manual: SessionStateResult = { id: 's1', state: 'WAITING_INPUT', mode: 'MANUAL', active: true }
const dead: SessionStateResult = { id: 's1', state: 'DISCONNECTED', mode: 'AUTO', active: false }

describe('awaitWaitingInput', () => {
  it('returns immediately when already ready', async () => {
    const result = await awaitWaitingInput({ client: mockClient([waiting]), sessionId: 's1', timeoutMs: 1000 })
    expect(result.state).toBe('WAITING_INPUT')
  })

  it('waits through CONNECTING then succeeds', async () => {
    const result = await awaitWaitingInput({
      client: mockClient([connecting, connecting, waiting]),
      sessionId: 's1',
      timeoutMs: 2000,
      pollMs: 10,
    })
    expect(result.state).toBe('WAITING_INPUT')
  })

  it('rejects MANUAL without waiting', async () => {
    await expect(awaitWaitingInput({ client: mockClient([manual]), sessionId: 's1', timeoutMs: 100 }))
      .rejects.toBeInstanceOf(SessionNotReadyError)
  })

  it('rejects DISCONNECTED', async () => {
    await expect(awaitWaitingInput({ client: mockClient([dead]), sessionId: 's1', timeoutMs: 100 }))
      .rejects.toMatchObject({ state: 'DISCONNECTED' })
  })

  it('times out while stuck in CONNECTING', async () => {
    await expect(awaitWaitingInput({
      client: mockClient([connecting]),
      sessionId: 's1',
      timeoutMs: 40,
      pollMs: 10,
    })).rejects.toMatchObject({ name: 'SessionNotReadyError' })
  })

  it('rejects when the session becomes MANUAL while waiting', async () => {
    await expect(awaitWaitingInput({
      client: mockClient([connecting, manual]),
      sessionId: 's1',
      timeoutMs: 200,
      pollMs: 5,
    })).rejects.toBeInstanceOf(SessionNotReadyError)
  })

  it('wakes on a session.state event and ignores unrelated events', async () => {
    let state: SessionStateResult = connecting
    const client: RpcClient = {
      request: async () => state,
      subscribe: async (_params, listener) => {
        queueMicrotask(() => {
          listener({ subscriptionId: 'sub', event: 'session.output', data: {} })
          state = waiting
          listener({ subscriptionId: 'sub', event: 'session.state', data: { sessionId: 's1', state: 'WAITING_INPUT' } })
        })
        return {
          subscriptionId: 'sub',
          initial: {},
          detach: () => undefined,
          unsubscribe: async () => undefined,
        }
      },
      close: () => undefined,
    }
    const result = await awaitWaitingInput({ client, sessionId: 's1', timeoutMs: 1000, pollMs: 200 })
    expect(result.state).toBe('WAITING_INPUT')
  })

  it('ignores a subscriber readState failure and still becomes ready', async () => {
    let calls = 0
    const client: RpcClient = {
      request: async () => {
        calls += 1
        if (calls === 1) return connecting
        if (calls === 2) throw new Error('flaky')
        return waiting
      },
      subscribe: async (_params, listener) => {
        listener({ subscriptionId: 'sub', event: 'session.state', data: {} })
        return {
          subscriptionId: 'sub',
          initial: {},
          detach: () => undefined,
          unsubscribe: async () => undefined,
        }
      },
      close: () => undefined,
    }
    const result = await awaitWaitingInput({ client, sessionId: 's1', timeoutMs: 1000, pollMs: 5 })
    expect(result.state).toBe('WAITING_INPUT')
  })
})
