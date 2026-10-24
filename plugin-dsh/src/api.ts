import type {
  DaemonJobSnapshot,
  ExecStartResult,
  HistoryResult,
  ProfileWriteInput,
  PublicProfile,
  RemoteEditResult,
  ShellinkClient,
  SessionCreateResult,
  SessionRow,
  SessionStateResult,
  TransferMeta,
} from './types.js'

export function listProfiles(client: ShellinkClient, query?: string): Promise<PublicProfile[]> {
  return client.request('profiles.list', query === undefined ? {} : { q: query })
}

export function getProfile(client: ShellinkClient, id: string): Promise<PublicProfile> {
  return client.request('profiles.get', { id })
}

export function createProfile(client: ShellinkClient, profile: ProfileWriteInput & { name: string }): Promise<PublicProfile> {
  return client.request('profiles.create', profile)
}

export function updateProfile(client: ShellinkClient, id: string, profile: ProfileWriteInput): Promise<PublicProfile> {
  return client.request('profiles.update', { id, profile })
}

export function deleteProfile(client: ShellinkClient, id: string): Promise<{ ok: true }> {
  return client.request('profiles.delete', { id })
}

export function listSessions(client: ShellinkClient): Promise<SessionRow[]> {
  return client.request('sessions.list', {})
}

export function createSession(client: ShellinkClient, profileId: string, cols?: number, rows?: number): Promise<SessionCreateResult> {
  return client.request('sessions.create', {
    profileId,
    ...cols !== undefined ? { cols } : {},
    ...rows !== undefined ? { rows } : {},
  })
}

export function sessionState(client: ShellinkClient, id: string): Promise<SessionStateResult> {
  return client.request('sessions.state', { id })
}

export function sessionHistory(client: ShellinkClient, id: string, since?: number, limit?: number): Promise<HistoryResult> {
  return client.request('sessions.history', {
    id,
    ...since !== undefined ? { since } : {},
    ...limit !== undefined ? { limit } : {},
  })
}

export function sessionInput(client: ShellinkClient, id: string, text: string, appendNewline = true): Promise<{ ok: true; state: string; lastSeq: number }> {
  return client.request('sessions.input', { id, text, appendNewline })
}

export function sessionExecStart(client: ShellinkClient, id: string, command: string, timeoutMs?: number): Promise<ExecStartResult> {
  return client.request('sessions.execStart', {
    id,
    command,
    ...timeoutMs !== undefined ? { timeoutMs } : {},
  })
}

export function sessionMode(client: ShellinkClient, id: string, mode: 'AUTO' | 'MANUAL'): Promise<{ ok: true; mode: string }> {
  return client.request('sessions.mode', { id, mode })
}

export function sessionClose(client: ShellinkClient, id: string): Promise<{ ok: true; note?: string }> {
  return client.request('sessions.close', { id })
}

export function sessionUploadStart(
  client: ShellinkClient,
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
  client: ShellinkClient,
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
  client: ShellinkClient,
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
