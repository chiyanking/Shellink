import { describe, expect, it } from 'vitest'
import { connectExisting } from '../src/daemon.js'
import { awaitWaitingInput } from '../src/ready.js'
import { waitForDaemonJob } from '../src/jobs.js'
import * as api from '../src/api.js'
import { shellinkPaths } from '@shellink/client'

/**
 * Optional live-daemon smoke. Skipped unless SHELLINK_DOCKER_E2E=1 and a
 * daemon is already listening (start one with `shellink server start` after
 * `npm run test:docker:up` in server/ if you need SSH fixtures).
 *
 * Uses a local `command` profile so the path does not require Docker SSH.
 */
describe.skipIf(process.env.SHELLINK_DOCKER_E2E !== '1')('live daemon open→exec→close', () => {
  it('opens a command session, execs, and closes', async () => {
    const client = await connectExisting(shellinkPaths().socketPath, 1000)
    expect(client, 'expected a running shellink daemon').not.toBeNull()
    if (!client) return

    const profile = await api.createProfile(client, {
      name: `dsh-e2e-${Date.now()}`,
      connectType: 'command',
      command: process.env.SHELL ?? '/bin/sh',
    })
    try {
      const created = await api.createSession(client, profile.id)
      const ready = await awaitWaitingInput({ client, sessionId: created.id, timeoutMs: 15_000, pollMs: 100 })
      expect(ready.state).toBe('WAITING_INPUT')
      const start = await api.sessionExecStart(client, created.id, 'echo shellink-dsh-e2e', 10_000)
      const result = await waitForDaemonJob(client, start.jobId, start.startSeq)
      expect(result.job.status).toBe('DONE')
      expect(result.output).toContain('shellink-dsh-e2e')
      const closed = await api.sessionClose(client, created.id)
      expect(closed.ok).toBe(true)
    } finally {
      await api.deleteProfile(client, profile.id).catch(() => undefined)
      client.close()
    }
  })
})
