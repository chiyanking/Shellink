import { describe, expect, it } from 'vitest'
import type { Context } from '@deepseek-ai/cordis'
import type { ToolRunContext } from '@deepseek-ai/dsh-tools'
import { apply } from '../src/tools.js'
import { Config } from '../src/config.js'
import { SessionTargetCache } from '../src/tier.js'
import type { DaemonJobSnapshot, ExecStartResult, HistoryResult, PublicProfile, SessionCreateResult, SessionRow, SessionStateResult } from '../src/types.js'
import type { RpcClient } from '../src/types.js'

interface RegisteredTool {
  name: string
  execute: (args: Record<string, unknown>, exec: ToolRunContext) => Promise<unknown>
  output?: {
    render: (args: Record<string, unknown>, value: Record<string, unknown>) => Array<{ type: string; text: string }>
  }
  presentCall?: (args: Record<string, unknown>) => unknown
  isConcurrencySafe?: (args: Record<string, unknown>) => boolean
}

interface Harness {
  ctx: Context
  tools: Map<string, RegisteredTool>
  calls: { method: string; args: unknown[] }[]
  jobStarts: Array<{ label: string; owner?: unknown; run: () => unknown }>
}

function profile(id = 'p1'): PublicProfile {
  return {
    id,
    name: 'dev',
    uniqueId: null,
    connectType: 'ssh',
    command: null,
    host: '127.0.0.1',
    port: 22,
    username: 'root',
    authType: 'key',
    hasPassword: false,
    hasPrivateKey: true,
    hasPassphrase: false,
    term: 'xterm',
    cols: 80,
    rows: 24,
    promptRegex: null,
    createdAt: 0,
    updatedAt: 0,
  }
}

function job(partial: Partial<DaemonJobSnapshot> = {}): DaemonJobSnapshot {
  return {
    id: 'job1',
    sessionId: 's1',
    kind: 'exec',
    command: 'uname',
    remotePath: null,
    startSeq: 1,
    startedAt: 0,
    endedAt: 1,
    timeoutMs: 20_000,
    status: 'DONE',
    state: 'WAITING_INPUT',
    error: null,
    result: null,
    ...partial,
  }
}

function fakeExec(agent = true): ToolRunContext {
  return {
    callId: 'call-1',
    rootCallId: 'call-1',
    name: 'shellink_session_exec',
    arguments: {},
    ...agent ? { agent: { id: 'agent-1' } } : {},
    signal: new AbortController().signal,
    deferContext: () => undefined,
    concludeTurn: () => undefined,
  } as unknown as ToolRunContext
}

function mockRpc(): RpcClient {
  return {
    request: async (method) => {
      if (method === 'sessions.execStatus') {
        return { job: job(), output: 'ok\n', cursor: 2, done: true }
      }
      throw new Error(method)
    },
    subscribe: async () => {
      throw new Error('unused')
    },
    close: () => undefined,
  }
}

function harness(options: {
  profileWrites?: boolean
  outcome?: 'allowed-once' | 'rejected'
  jobs?: boolean
  uploadMaxBytes?: number
  fileSize?: number
  fileBytes?: Uint8Array
  fail?: string
} = {}): Harness {
  const tools = new Map<string, RegisteredTool>()
  const calls: Harness['calls'] = []
  const jobStarts: Harness['jobStarts'] = []
  const settings = Config()
  settings.profileWrites = options.profileWrites ?? true
  settings.approval.mode = options.outcome === undefined ? 'off' : 'enforce'
  settings.approval.gateProfileDelete = true
  settings.approval.gateSessionClose = true
  if (options.uploadMaxBytes !== undefined) settings.uploadMaxBytes = options.uploadMaxBytes
  const record = <T>(method: string, value: T, ...args: unknown[]): T => {
    calls.push({ method, args })
    if (options.fail === method) throw new Error(`${method} failed`)
    return value
  }
  const fileBytes = options.fileBytes ?? new Uint8Array([1, 2, 3])
  const ctx = {
    shellink: {
      settings,
      sessionTargets: new SessionTargetCache(),
      ensureClient: async () => mockRpc(),
      pollJob: async () => ({ status: 'completed', output: 'bg' }),
      listProfiles: async (query?: string) => record('listProfiles', [profile()], query),
      getProfile: async (id: string) => record('getProfile', profile(id), id),
      createProfile: async (input: unknown) => record('createProfile', profile(), input),
      updateProfile: async (id: string, input: unknown) => record('updateProfile', profile(id), id, input),
      deleteProfile: async (id: string) => record('deleteProfile', { ok: true as const }, id),
      listSessions: async () => record('listSessions', [] as SessionRow[]),
      openSession: async (id: string, cols?: number, rows?: number) => record('openSession', {
        id: 's1',
        state: 'WAITING_INPUT',
        mode: 'AUTO',
        target: '127.0.0.1',
      } satisfies SessionCreateResult, id, cols, rows),
      sessionState: async (id: string) => record('sessionState', {
        id,
        state: 'WAITING_INPUT',
        mode: 'MANUAL',
        active: true,
      } satisfies SessionStateResult, id),
      sessionHistory: async (id: string, since?: number, limit?: number) => record('sessionHistory', {
        cursor: 3,
        text: 'hi',
      } satisfies HistoryResult, id, since, limit),
      sessionInput: async (id: string, text: string, nl: boolean) => record('sessionInput', {
        ok: true as const,
        state: 'WAITING_INPUT',
        lastSeq: 1,
      }, id, text, nl),
      execStart: async (id: string, command: string, timeout?: number) => record('execStart', {
        jobId: 'job1',
        status: 'RUNNING',
        startSeq: 1,
        state: 'OUTPUTTING',
      } satisfies ExecStartResult, id, command, timeout),
      sessionMode: async (id: string, mode: string) => record('sessionMode', { ok: true as const, mode }, id, mode),
      sessionClose: async (id: string) => record('sessionClose', { ok: true as const }, id),
      uploadStart: async (id: string, path: string, data: Uint8Array) => record('uploadStart', job({
        kind: 'upload',
        id: 'up1',
        remotePath: path,
      }), id, path, data),
      downloadStart: async (id: string, remote: string, local: string) => record('downloadStart', job({
        kind: 'download',
        id: 'dl1',
        remotePath: remote,
      }), id, remote, local),
      editStart: async (id: string, path: string, edits: unknown) => record('editStart', job({
        kind: 'edit',
        id: 'ed1',
        remotePath: path,
      }), id, path, edits),
    },
    systemPrompt: { section: () => undefined },
    tools: {
      register: (tool: RegisteredTool) => {
        tools.set(tool.name, tool)
      },
    },
    fs: {
      resolve: async (path: string) => path,
      stat: async () => options.fileSize === undefined ? null : { size: options.fileSize },
      readBytes: async () => fileBytes,
      processPath: (path: string) => `/abs${path}`,
    },
    get: (name: string) => {
      if (name === 'approval' && options.outcome !== undefined) {
        return { request: async () => options.outcome }
      }
      if (name === 'jobs' && options.jobs) {
        return {
          start: (spec: { label: string; owner?: unknown; run: () => unknown }) => {
            jobStarts.push(spec)
            return 99
          },
        }
      }
      return undefined
    },
    effect: () => () => undefined,
  }
  apply(ctx as unknown as Context)
  return { ctx: ctx as unknown as Context, tools, calls, jobStarts }
}

async function run(tools: Map<string, RegisteredTool>, name: string, args: Record<string, unknown>, exec = fakeExec()) {
  const tool = tools.get(name)
  if (!tool) throw new Error(`missing ${name}`)
  return tool.execute(args, exec)
}

describe('tool execute paths', () => {
  it('covers read tools, presenters, and renderers', async () => {
    const { tools } = harness()
    expect(await run(tools, 'shellink_profile_list', { query: 'prod' })).toEqual({ profiles: [profile()] })
    expect(await run(tools, 'shellink_profile_get', { profile_id: 'p1' })).toEqual({ profile: profile('p1') })
    expect(await run(tools, 'shellink_session_list', {})).toEqual({ sessions: [] })
    expect(await run(tools, 'shellink_session_state', { session_id: 's1' })).toMatchObject({ state: { id: 's1' } })
    expect(await run(tools, 'shellink_session_history', { session_id: 's1', since: 1, limit: 10 }))
      .toEqual({ cursor: 3, text: 'hi' })
    const presentArgs: Record<string, Record<string, unknown>> = {
      shellink_profile_list: { query: 'prod' },
      shellink_profile_get: { profile_id: 'p1' },
      shellink_profile_create: { name: 'web' },
      shellink_profile_update: { profile_id: 'p1' },
      shellink_profile_delete: { profile_id: 'p1' },
      shellink_session_list: {},
      shellink_session_open: { profile_id: 'p1' },
      shellink_session_state: { session_id: 's1' },
      shellink_session_history: { session_id: 's1' },
      shellink_session_input: { session_id: 's1', text: 'y' },
      shellink_session_exec: { session_id: 's1', command: 'uname' },
      shellink_session_mode: { session_id: 's1', mode: 'AUTO' },
      shellink_session_close: { session_id: 's1' },
      shellink_session_upload: { session_id: 's1', local_path: '/tmp/a', remote_path: '/tmp/b' },
      shellink_session_download: { session_id: 's1', remote_path: '/tmp/b', local_path: '/tmp/out' },
      shellink_session_edit: { session_id: 's1', path: '/tmp/b', edits: [{ old_text: 'a', new_text: 'b' }] },
    }
    const renderValues: Record<string, Record<string, unknown>> = {
      shellink_profile_list: { profiles: [{ id: 'p1' }] },
      shellink_profile_get: { profile: { id: 'p1' } },
      shellink_profile_create: { profile: { id: 'p1' } },
      shellink_profile_update: { profile: { id: 'p1' } },
      shellink_profile_delete: { ok: true, profile_id: 'p1' },
      shellink_session_list: { sessions: [] },
      shellink_session_open: { session: { id: 's1' } },
      shellink_session_state: { state: { id: 's1' } },
      shellink_session_history: { cursor: 2, text: 'out' },
      shellink_session_input: { ok: true },
      shellink_session_exec: { kind: 'foreground', jobId: '9', status: 'DONE', output: 'ok', error: null },
      shellink_session_mode: { ok: true, mode: 'AUTO' },
      shellink_session_close: { ok: true },
      shellink_session_upload: { kind: 'foreground', jobId: 'u', status: 'DONE', output: 'ok' },
      shellink_session_download: { kind: 'foreground', jobId: 'd', status: 'DONE', output: 'ok' },
      shellink_session_edit: { kind: 'foreground', jobId: 'e', status: 'DONE', output: 'ok' },
    }
    for (const tool of tools.values()) {
      const args = presentArgs[tool.name] ?? {}
      expect(tool.presentCall?.(args)).toBeDefined()
      tool.isConcurrencySafe?.(args)
      const rendered = tool.output?.render(args, renderValues[tool.name] ?? {})
      expect(rendered?.[0]?.type).toBe('text')
    }
    const exec = tools.get('shellink_session_exec')
    expect(exec?.output?.render(presentArgs.shellink_session_exec ?? {}, { kind: 'background', jobId: '9' })[0]?.text)
      .toContain('background job 9')
    expect(exec?.output?.render(presentArgs.shellink_session_exec ?? {}, {
      kind: 'foreground',
      jobId: '9',
      status: 'DONE',
      output: 'ok',
      error: 'e',
    })[0]?.text).toContain('error: e')
    for (const name of ['shellink_session_upload', 'shellink_session_download', 'shellink_session_edit']) {
      const tool = tools.get(name)
      expect(tool?.output?.render(presentArgs[name] ?? {}, { kind: 'background', jobId: 'bg' })[0]?.text)
        .toContain('background')
    }
  })

  it('creates and updates profiles with optional fields', async () => {
    const { tools, calls } = harness()
    await run(tools, 'shellink_profile_create', {
      name: 'web',
      connect_type: 'ssh',
      host: 'h',
      port: 22,
      username: 'root',
      auth_type: 'key',
      password_ref: 'PW',
      private_key_ref: 'KEY',
      passphrase_ref: 'PH',
      term: 'xterm',
      cols: 80,
      rows: 24,
      prompt_regex: '#',
      unique_id: 'guid',
    })
    await run(tools, 'shellink_profile_create', { name: ' hop', connect_type: 'command', command: 'expect x' })
    await run(tools, 'shellink_profile_update', {
      profile_id: 'p1',
      name: 'web2',
      connect_type: 'ssh',
      host: 'h2',
      port: 2222,
      username: 'u',
      auth_type: 'password',
      password_ref: 'PW',
      private_key_ref: 'KEY',
      passphrase_ref: 'PH',
      term: 'vt100',
      cols: 100,
      rows: 40,
      prompt_regex: '$',
      unique_id: 'guid2',
      command: 'unused',
    })
    await run(tools, 'shellink_profile_update', { profile_id: 'p1' })
    expect(calls.some(c => c.method === 'createProfile')).toBe(true)
    expect(calls.some(c => c.method === 'updateProfile')).toBe(true)
  })

  it('opens, inputs, execs, modes, and closes a session', async () => {
    const { tools, calls } = harness()
    await run(tools, 'shellink_session_open', { profile_id: 'p1', cols: 80, rows: 24 })
    await run(tools, 'shellink_session_input', { session_id: 's1', text: 'y' })
    const fg = await run(tools, 'shellink_session_exec', { session_id: 's1', command: 'uname -a', timeout_ms: 5000 })
    expect(fg).toMatchObject({ kind: 'foreground', status: 'DONE', output: 'ok\n' })
    await run(tools, 'shellink_session_mode', { session_id: 's1', mode: 'AUTO' })
    await run(tools, 'shellink_session_mode', { session_id: 's1', mode: 'MANUAL' })
    await run(tools, 'shellink_session_close', { session_id: 's1' })
    await run(tools, 'shellink_profile_delete', { profile_id: 'p1' })
    expect(calls.map(c => c.method)).toEqual(expect.arrayContaining([
      'openSession',
      'sessionInput',
      'execStart',
      'sessionMode',
      'sessionClose',
      'deleteProfile',
    ]))
  })

  it('starts background jobs when the jobs service is present', async () => {
    const { tools, jobStarts } = harness({ jobs: true })
    const started = await run(tools, 'shellink_session_exec', {
      session_id: 's1',
      command: 'sleep 10',
      run_in_background: true,
    })
    expect(started).toEqual({ kind: 'background', jobId: '99' })
    expect(jobStarts[0]?.label).toBe('sleep 10')
    expect(jobStarts[0]?.owner).toEqual({ id: 'agent-1' })
    await jobStarts[0]?.run()
    const noAgent = await run(tools, 'shellink_session_exec', {
      session_id: 's1',
      command: 'sleep 11',
      run_in_background: true,
    }, fakeExec(false))
    expect(noAgent).toEqual({ kind: 'background', jobId: '99' })
    expect(jobStarts[1]?.owner).toBeUndefined()
  })

  it('rejects background work when jobs is not composed', async () => {
    const { tools } = harness()
    await expect(run(tools, 'shellink_session_exec', {
      session_id: 's1',
      command: 'sleep 10',
      run_in_background: true,
    })).rejects.toThrow(/background jobs unavailable/)
  })

  it('uploads, downloads, and edits through the job helper', async () => {
    const { tools, calls } = harness({ fileSize: 3 })
    const upload = await run(tools, 'shellink_session_upload', {
      session_id: 's1',
      local_path: '/tmp/a',
      remote_path: '/tmp/b',
    })
    expect(upload).toMatchObject({ kind: 'foreground', jobId: 'job1' })
    const download = await run(tools, 'shellink_session_download', {
      session_id: 's1',
      remote_path: '/tmp/b',
      local_path: '/tmp/out',
    })
    expect(download).toMatchObject({ kind: 'foreground', jobId: 'job1' })
    const edited = await run(tools, 'shellink_session_edit', {
      session_id: 's1',
      path: '/tmp/b',
      edits: [{ old_text: 'a', new_text: 'b' }],
    })
    expect(edited).toMatchObject({ kind: 'foreground', jobId: 'job1' })
    expect(calls.some(c => c.method === 'uploadStart')).toBe(true)
    expect(calls.some(c => c.method === 'downloadStart')).toBe(true)
    expect(calls.some(c => c.method === 'editStart')).toBe(true)
  })

  it('rejects oversized uploads from stat and from the byte payload', async () => {
    const tooBig = harness({ fileSize: 100, uploadMaxBytes: 10 })
    await expect(run(tooBig.tools, 'shellink_session_upload', {
      session_id: 's1',
      local_path: '/tmp/a',
      remote_path: '/tmp/b',
    })).rejects.toThrow(/uploadMaxBytes/)

    const payload = harness({ fileSize: 1, uploadMaxBytes: 2, fileBytes: new Uint8Array([1, 2, 3]) })
    await expect(run(payload.tools, 'shellink_session_upload', {
      session_id: 's1',
      local_path: '/tmp/a',
      remote_path: '/tmp/b',
    })).rejects.toThrow(/uploadMaxBytes/)
  })

  it('rejects empty edits', async () => {
    const { tools } = harness()
    await expect(run(tools, 'shellink_session_edit', {
      session_id: 's1',
      path: '/tmp/b',
      edits: [],
    })).rejects.toThrow(/at least one replacement/)
  })

  it('wraps service failures for the model', async () => {
    const { tools } = harness({ fail: 'listProfiles' })
    await expect(run(tools, 'shellink_profile_list', { query: 'x' })).rejects.toThrow(/listProfiles failed/)
  })

  it('asks for dangerous exec when approval is on, and still starts after allow-once', async () => {
    const { tools, calls } = harness({ outcome: 'allowed-once' })
    await run(tools, 'shellink_session_exec', { session_id: 's1', command: 'rm -rf /tmp/x' })
    expect(calls.some(c => c.method === 'execStart')).toBe(true)
  })
})
