import type { ClientSubscription } from '@shellink/client'
import type { RpcEvent } from '@shellink/protocol'

export type SessionState = 'CONNECTING' | 'OUTPUTTING' | 'IDLE' | 'WAITING_INPUT' | 'DISCONNECTED'
export type InteractionMode = 'AUTO' | 'MANUAL'
export type DaemonJobStatus = 'RUNNING' | 'DONE' | 'TIMED_OUT' | 'CANCELED' | 'DISCONNECTED' | 'FAILED'
export type DaemonJobKind = 'exec' | 'upload' | 'download' | 'edit'

export interface RpcClient {
  request<T = unknown>(method: string, params?: unknown, timeoutMs?: number): Promise<T>
  subscribe(params: { sessionId?: string; replay?: boolean }, listener: (event: RpcEvent) => void): Promise<ClientSubscription>
  close(): void
}

export interface PublicProfile {
  id: string
  name: string
  uniqueId: string | null
  connectType: 'ssh' | 'command'
  command: string | null
  host: string
  port: number
  username: string
  authType: string
  hasPassword: boolean
  hasPrivateKey: boolean
  hasPassphrase: boolean
  term: string
  cols: number
  rows: number
  promptRegex: string | null
  createdAt: number
  updatedAt: number
}

export interface SessionRow {
  id: string
  profileId: string
  profileName: string
  target: string
  state: SessionState
  mode: InteractionMode
  cols: number
  rows: number
  createdAt: number
  closedAt: number | null
  closeReason: string | null
  active: boolean
}

export interface SessionCreateResult {
  id: string
  state: SessionState
  mode: InteractionMode
  target: string
}

export interface SessionStateResult {
  id: string
  state: SessionState
  mode: InteractionMode
  lastSeq?: number
  recentOutput?: string
  closeReason?: string | null
  exitCode?: number | null
  active: boolean
}

export interface HistoryResult {
  cursor: number
  text: string
}

export interface DaemonJobSnapshot {
  id: string
  sessionId: string
  kind: DaemonJobKind
  command: string | null
  remotePath: string | null
  startSeq: number
  startedAt: number
  endedAt: number | null
  timeoutMs: number
  status: DaemonJobStatus
  state: string | null
  error: string | null
  result: unknown | null
}

export interface ExecStartResult {
  jobId: string
  status: DaemonJobStatus
  startSeq: number
  state: string
}

export interface ExecStatusResult {
  job: DaemonJobSnapshot
  output: string
  cursor: number
  done: boolean
}

export interface TransferMeta {
  remotePath: string
  size: number
  sha256: string
  codec: string
  durationMs: number
}

export interface RemoteEditResult {
  ok: true
  path: string
  engine: string
  replaced: number
  durationMs: number
}

export interface ProfileWriteInput {
  name?: string
  uniqueId?: string | null
  connectType?: 'ssh' | 'command'
  command?: string | null
  host?: string
  port?: number
  username?: string
  authType?: 'password' | 'key'
  password?: string
  privateKey?: string
  passphrase?: string
  term?: string
  cols?: number
  rows?: number
  promptRegex?: string | null
}

export interface ProfileSecretRefs {
  passwordRef?: string
  privateKeyRef?: string
  passphraseRef?: string
}

/** Daemon job statuses that mean the producer has released the session lock. */
export const DAEMON_JOB_SETTLED = new Set<DaemonJobStatus>(['DONE', 'CANCELED', 'DISCONNECTED', 'FAILED'])
