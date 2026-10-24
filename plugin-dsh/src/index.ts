/**
 * Shellink HTTP client as a Cordis service (`ctx.shellink`).
 * Dispose closes only this process's client; the daemon and remote sessions stay up.
 * @module shellink-dsh
 */

import { Context, Service } from '@deepseek-ai/cordis'
import { credentialRef } from '@deepseek-ai/dsh-credentials'
import type { ShellinkHttpClient } from './client.js'
import * as api from './api.js'
import { Config } from './config.js'
import type { Config as ConfigValue } from './config.js'
import { resolveProfileSecrets } from './credentials.js'
import { ensureDaemon, resolveBaseUrl } from './daemon.js'
import { formatRpcError } from './errors.js'
import { pollDaemonJob, waitForDaemonJob } from './jobs.js'
import { awaitWaitingInput } from './ready.js'
import { SessionTargetCache } from './tier.js'
import type {
  DaemonJobSnapshot,
  ExecStartResult,
  HistoryResult,
  ProfileSecretRefs,
  ProfileWriteInput,
  PublicProfile,
  ShellinkClient,
  SessionCreateResult,
  SessionRow,
  SessionStateResult,
} from './types.js'

export { Config } from './config.js'
export type { Config as ShellinkConfig } from './config.js'
export {
  MissingCredentialError,
  SessionNotReadyError,
  ShellinkBinaryMissingError,
  formatRpcError,
} from './errors.js'

export const name = 'shellink'
export const inject = ['credentials']

declare module '@deepseek-ai/cordis' {
  interface Context {
    shellink: ShellinkService
  }
}

/** Host-side Shellink HTTP service. */
export class ShellinkService extends Service {
  static inject = ['credentials']
  static Config = Config

  private client: ShellinkHttpClient | null = null
  private connecting: Promise<ShellinkClient> | null = null
  readonly settings: ConfigValue
  readonly sessionTargets = new SessionTargetCache()

  constructor(ctx: Context, config: ConfigValue) {
    super(ctx, 'shellink')
    this.settings = config
    ctx.effect(() => () => {
      this.client?.close()
      this.client = null
      this.connecting = null
      this.sessionTargets.clear()
    }, 'shellink HTTP client teardown')
  }

  /** Lazy-connect to the daemon, starting it when configured. */
  async ensureClient(): Promise<ShellinkClient> {
    if (this.client) return this.client
    if (this.connecting) return this.connecting
    this.connecting = (async () => {
      const client = await ensureDaemon({
        baseUrl: resolveBaseUrl(this.settings.baseUrl),
        autoStart: this.settings.autoStartDaemon,
        timeoutMs: this.settings.daemonReadyTimeoutMs,
        requestTimeoutMs: this.settings.requestTimeoutMs,
      })
      this.client = client
      return client
    })()
    try {
      return await this.connecting
    } catch (error) {
      this.connecting = null
      throw error
    }
  }

  async awaitReady(sessionId: string): Promise<SessionStateResult> {
    const client = await this.ensureClient()
    return awaitWaitingInput({
      client,
      sessionId,
      timeoutMs: this.settings.waitReadyTimeoutMs,
    })
  }

  async listProfiles(query?: string): Promise<PublicProfile[]> {
    return api.listProfiles(await this.ensureClient(), query)
  }

  async getProfile(id: string): Promise<PublicProfile> {
    return api.getProfile(await this.ensureClient(), id)
  }

  async createProfile(input: ProfileWriteInput & ProfileSecretRefs & { name: string }): Promise<PublicProfile> {
    const secrets = await this.resolveSecrets(input)
    return api.createProfile(await this.ensureClient(), { ...withoutRefs(input), ...secrets })
  }

  async updateProfile(id: string, input: ProfileWriteInput & ProfileSecretRefs): Promise<PublicProfile> {
    const secrets = await this.resolveSecrets(input)
    return api.updateProfile(await this.ensureClient(), id, { ...withoutRefs(input), ...secrets })
  }

  async deleteProfile(id: string): Promise<{ ok: true }> {
    return api.deleteProfile(await this.ensureClient(), id)
  }

  async listSessions(): Promise<SessionRow[]> {
    return api.listSessions(await this.ensureClient())
  }

  async openSession(profileId: string, cols?: number, rows?: number): Promise<SessionCreateResult> {
    const created = await api.createSession(await this.ensureClient(), profileId, cols, rows)
    this.sessionTargets.set(created.id, {
      profileId,
      profileName: '',
      target: created.target,
    })
    await this.awaitReady(created.id)
    return created
  }

  async sessionState(id: string): Promise<SessionStateResult> {
    return api.sessionState(await this.ensureClient(), id)
  }

  async sessionHistory(id: string, since?: number, limit?: number): Promise<HistoryResult> {
    return api.sessionHistory(await this.ensureClient(), id, since, limit)
  }

  async sessionInput(id: string, text: string, appendNewline = true): Promise<{ ok: true; state: string; lastSeq: number }> {
    return api.sessionInput(await this.ensureClient(), id, text, appendNewline)
  }

  async execStart(id: string, command: string, timeoutMs?: number): Promise<ExecStartResult> {
    await this.awaitReady(id)
    return api.sessionExecStart(await this.ensureClient(), id, command, timeoutMs ?? this.settings.execDefaultTimeoutMs)
  }

  async execForeground(id: string, command: string, timeoutMs?: number, signal?: AbortSignal): Promise<{ jobId: string; status: string; output: string; error: string | null }> {
    const start = await this.execStart(id, command, timeoutMs)
    const result = await waitForDaemonJob(await this.ensureClient(), start.jobId, start.startSeq, signal)
    return {
      jobId: result.job.id,
      status: result.job.status,
      output: result.output,
      error: result.job.error,
    }
  }

  pollJob(jobId: string, startSeq: number) {
    if (!this.client) throw new Error('not connected to the shellink daemon')
    return pollDaemonJob(this.client, jobId, startSeq)
  }

  async uploadStart(id: string, remotePath: string, data: Uint8Array): Promise<DaemonJobSnapshot> {
    await this.awaitReady(id)
    return api.sessionUploadStart(await this.ensureClient(), id, remotePath, data)
  }

  async downloadStart(id: string, remotePath: string, output: string): Promise<DaemonJobSnapshot> {
    await this.awaitReady(id)
    return api.sessionDownloadStart(await this.ensureClient(), id, remotePath, output)
  }

  async editStart(id: string, remotePath: string, edits: Array<{ oldText: string; newText: string }>): Promise<DaemonJobSnapshot> {
    await this.awaitReady(id)
    return api.sessionEditStart(await this.ensureClient(), id, remotePath, edits)
  }

  async sessionMode(id: string, mode: 'AUTO' | 'MANUAL'): Promise<{ ok: true; mode: string }> {
    return api.sessionMode(await this.ensureClient(), id, mode)
  }

  async sessionClose(id: string): Promise<{ ok: true; note?: string }> {
    const result = await api.sessionClose(await this.ensureClient(), id)
    this.sessionTargets.delete(id)
    return result
  }

  private async resolveSecrets(refs: ProfileSecretRefs): Promise<{ password?: string; privateKey?: string; passphrase?: string }> {
    return resolveProfileSecrets({
      resolve: async (ref) => {
        const resolved = await this.ctx.credentials.resolve(credentialRef(ref))
        return resolved?.value
      },
    }, refs)
  }
}

function withoutRefs<T extends ProfileWriteInput & ProfileSecretRefs>(input: T): ProfileWriteInput & { name?: string } {
  const { passwordRef: _p, privateKeyRef: _k, passphraseRef: _s, ...rest } = input
  return rest
}

export { formatRpcError as formatShellinkError }
export default ShellinkService
