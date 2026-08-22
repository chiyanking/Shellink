import type { RpcEvent } from '@shellink/protocol'
import { SessionNotReadyError, notReadyMessage } from './errors.js'
import type { InteractionMode, RpcClient, SessionState, SessionStateResult } from './types.js'

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function isReady(state: SessionState, mode: InteractionMode): boolean {
  return mode === 'AUTO' && state === 'WAITING_INPUT'
}

/** Wait until a session is AUTO + WAITING_INPUT, or throw an actionable error. */
export async function awaitWaitingInput(options: {
  client: RpcClient
  sessionId: string
  timeoutMs: number
  pollMs?: number
}): Promise<SessionStateResult> {
  const { client, sessionId, timeoutMs, pollMs = 200 } = options
  const readState = () => client.request<SessionStateResult>('sessions.state', { id: sessionId })

  const initial = await readState()
  if (initial.mode === 'MANUAL' || initial.state === 'DISCONNECTED') {
    throw new SessionNotReadyError(sessionId, initial.state, initial.mode, notReadyMessage(sessionId, initial.state, initial.mode))
  }
  if (isReady(initial.state, initial.mode)) return initial

  const deadline = Date.now() + timeoutMs
  let latest = initial
  let settled: ((state: SessionStateResult) => void) | undefined
  const sub = await client.subscribe({ sessionId }, (event: RpcEvent) => {
    if (event.event !== 'session.state' && event.event !== 'session.mode' && event.event !== 'session.closed') return
    void readState().then((state) => {
      latest = state
      settled?.(state)
    }).catch(() => undefined)
  })

  try {
    while (Date.now() < deadline) {
      latest = await readState()
      if (latest.mode === 'MANUAL' || latest.state === 'DISCONNECTED') {
        throw new SessionNotReadyError(sessionId, latest.state, latest.mode, notReadyMessage(sessionId, latest.state, latest.mode))
      }
      if (isReady(latest.state, latest.mode)) return latest
      const remaining = Math.max(0, deadline - Date.now())
      await Promise.race([
        sleep(Math.min(pollMs, remaining || pollMs)),
        new Promise<void>((resolve) => {
          settled = () => resolve()
        }),
      ])
    }
    throw new SessionNotReadyError(sessionId, latest.state, latest.mode, `${notReadyMessage(sessionId, latest.state, latest.mode)} Timed out after ${timeoutMs}ms.`)
  } finally {
    await sub.unsubscribe().catch(() => undefined)
  }
}
