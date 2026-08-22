import { DAEMON_JOB_SETTLED, type DaemonJobSnapshot, type ExecStatusResult, type RpcClient } from './types.js'

export interface JobPollHooks {
  cancel(reason?: string): void
  done: Promise<{ status: 'completed' | 'killed' | 'failed'; detail?: string; output?: string }>
  readOutput(): string
}

/** Poll `sessions.execStatus` until the daemon job settles. TIMED_OUT is not settled. */
export function pollDaemonJob(client: RpcClient, jobId: string, startSeq: number, waitMs = 20_000): JobPollHooks {
  let cursor = startSeq
  let buffer = ''
  let canceled = false

  const done = (async () => {
    while (!canceled) {
      const status = await client.request<ExecStatusResult>('sessions.execStatus', { jobId, since: cursor, waitMs })
      if (status.output) buffer += status.output
      cursor = status.cursor
      if (status.done || DAEMON_JOB_SETTLED.has(status.job.status)) {
        return outcomeOf(status.job, buffer)
      }
    }
    return { status: 'killed' as const, output: buffer }
  })()

  return {
    cancel: () => {
      canceled = true
      void client.request('sessions.execCancel', { jobId }).catch(() => undefined)
    },
    done,
    readOutput: () => buffer,
  }
}

/** Wait for a daemon job to settle and return the accumulated output. */
export async function waitForDaemonJob(
  client: RpcClient,
  jobId: string,
  startSeq: number,
  signal?: AbortSignal,
): Promise<{ job: DaemonJobSnapshot; output: string }> {
  const hooks = pollDaemonJob(client, jobId, startSeq)
  const abort = () => hooks.cancel('aborted')
  signal?.addEventListener('abort', abort, { once: true })
  try {
    await hooks.done
    const last = await client.request<ExecStatusResult>('sessions.execStatus', { jobId, since: 0, waitMs: 0 })
    return { job: last.job, output: hooks.readOutput() || last.output }
  } finally {
    signal?.removeEventListener('abort', abort)
  }
}

function outcomeOf(job: DaemonJobSnapshot, output: string): { status: 'completed' | 'killed' | 'failed'; detail?: string; output?: string } {
  if (job.status === 'DONE') return { status: 'completed', output }
  if (job.status === 'CANCELED') return { status: 'killed', output }
  return { status: 'failed', detail: job.error ?? job.status, output }
}
