import type {
  DaemonJobSnapshot,
  ExecStartResult,
  HistoryResult,
  ProfileWriteInput,
  PublicProfile,
  RemoteEditResult,
  RpcClient,
  SessionCreateResult,
  SessionRow,
  SessionStateResult,
  TransferMeta,
} from './types.js'

export function listProfiles(client: RpcClient, query?: string): Promise<PublicProfile[]> {
  return client.request('profiles.list', query === undefined ? {} : { q: query })
}

export function getProfile(client: RpcClient, id: string): Promise<PublicProfile> {
  return client.request('profiles.get', { id })
}

export function createProfile(client: RpcClient, profile: ProfileWriteInput & { name: string }): Promise<PublicProfile> {
  return client.request('profiles.create', profile)
}

export function updateProfile(client: RpcClient, id: string, profile: ProfileWriteInput): Promise<PublicProfile> {
  return client.request('profiles.update', { id, profile })
}

export function deleteProfile(client: RpcClient, id: string): Promise<{ ok: true }> {
  return client.request('profiles.delete', { id })
}

export function listSessions(client: RpcClient): Promise<SessionRow[]> {
  return client.request('sessions.list', {})
}

export function createSession(client: RpcClient, profileId: string, cols?: number, rows?: number): Promise<SessionCreateResult> {
  return client.request('sessions.create', {
    profileId,
    ...cols !== undefined ? { cols } : {},
    ...rows !== undefined ? { rows } : {},
  })
}

export function sessionState(client: RpcClient, id: string): Promise<SessionStateResult> {
  return client.request('sessions.state', { id })
}

export function sessionHistory(client: RpcClient, id: string, since?: number, limit?: number): Promise<HistoryResult> {
  return client.request('sessions.history', {
    id,
    ...since !== undefined ? { since } : {},
    ...limit !== undefined ? { limit } : {},
  })
}

export function sessionInput(client: RpcClient, id: string, text: string, appendNewline = true): Promise<{ ok: true; state: string; lastSeq: number }> {
  return client.request('sessions.input', { id, text, appendNewline })
}

export function sessionExecStart(client: RpcClient, id: string, command: string, timeoutMs?: number): Promise<ExecStartResult> {
  return client.request('sessions.execStart', {
    id,
    command,
    ...timeoutMs !== undefined ? { timeoutMs } : {},
  })
}

export function sessionMode(client: RpcClient, id: string, mode: 'AUTO' | 'MANUAL'): Promise<{ ok: true; mode: string }> {
  return client.request('sessions.mode', { id, mode })
}

export function sessionClose(client: RpcClient, id: string): Promise<{ ok: true; note?: string }> {
  return client.request('sessions.close', { id })
}

export function sessionUploadStart(
  client: RpcClient,
  id: string,
  remotePath: string,
  data: Uint8Array,
  timeoutMs?: number,
): Promise<DaemonJobSnapshot> {
  return client.request('sessions.uploadStart', {
    id,
    path: remotePath,
    data,
    ...timeoutMs !== undefined ? { timeoutMs } : {},
  })
}

export function sessionDownloadStart(
  client: RpcClient,
  id: string,
  remotePath: string,
  output: string,
  timeoutMs?: number,
): Promise<DaemonJobSnapshot> {
  return client.request('sessions.downloadStart', {
    id,
    path: remotePath,
    output,
    ...timeoutMs !== undefined ? { timeoutMs } : {},
  })
}

export function sessionEditStart(
  client: RpcClient,
  id: string,
  remotePath: string,
  edits: Array<{ oldText: string; newText: string }>,
  timeoutMs?: number,
): Promise<DaemonJobSnapshot> {
  return client.request('sessions.editStart', {
    id,
    path: remotePath,
    edits,
    ...timeoutMs !== undefined ? { timeoutMs } : {},
  })
}

export type { TransferMeta, RemoteEditResult }
