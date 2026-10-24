import { describe, expect, it } from 'vitest'
import { SessionNotReadyError } from '../src/errors.js'
import { awaitWaitingInput } from '../src/ready.js'
import type { ShellinkClient, SessionStateResult } from '../src/types.js'

function mockClient(states: SessionStateResult[]): ShellinkClient {
  let index = 0
  return {
    request: async (method) => {
      if (method !== 'sessions.state') throw new Error(method)
      const current = states[Math.min(index, states.length - 1)]!
      if (index < states.length - 1) index++
      return current
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

  it('polls state until the session is ready', async () => {
    let calls = 0
    const client: ShellinkClient = {
      request: async () => {
        calls += 1
        if (calls === 1) return connecting
        return waiting
      },
      close: () => undefined,
    }
    const result = await awaitWaitingInput({ client, sessionId: 's1', timeoutMs: 1000, pollMs: 5 })
    expect(result.state).toBe('WAITING_INPUT')
    expect(calls).toBe(2)
  })
})
