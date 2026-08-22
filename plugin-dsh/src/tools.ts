/**
 * Model-facing Shellink tools. All remote work goes through `ctx.shellink`.
 * @module shellink-dsh/tools
 */

import type { Context } from '@deepseek-ai/cordis'
import type { ToolRunContext } from '@deepseek-ai/dsh-tools'
import { defineTool } from '@deepseek-ai/dsh-tools'
import { ApprovalGate } from './approval.js'
import { formatRpcError } from './errors.js'
import { waitForDaemonJob } from './jobs.js'
import { SHELLINK_PROMPT } from './prompt.js'
import type { DaemonJobSnapshot, ExecStartResult } from './types.js'

import type {} from '@deepseek-ai/dsh-fs'
import type {} from '@deepseek-ai/dsh-jobs'
import type {} from '@deepseek-ai/dsh-system-prompt'
import type { ShellinkService } from './index.js'

declare module '@deepseek-ai/dsh-jobs' {
  interface JobKindMap {
    shellink: 'shellink'
  }
}

export const name = 'shellink-tools'
export const inject = ['shellink', 'tools', 'systemPrompt', 'fs']

const BACKGROUND_OUTPUT = {
  kind: { type: 'string', required: true, const: 'background' },
  jobId: { type: 'string', required: true },
} as const

const OPAQUE_OBJECT = { type: 'object', additionalProperties: true } as const
const REQUIRED_OPAQUE_OBJECT = { type: 'object', required: true, additionalProperties: true } as const

const FOREGROUND_JOB_OUTPUT = {
  kind: { type: 'string', required: true, const: 'foreground' },
  jobId: { type: 'string', required: true },
  status: { type: 'string', required: true },
  output: { type: 'string', required: true },
  error: { required: true, oneOf: [{ type: 'string' }, { type: 'null' }] },
} as const

function json(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

function asError(error: unknown): never {
  throw new Error(formatRpcError(error))
}

async function runStartedJob(
  ctx: Context,
  exec: ToolRunContext,
  background: boolean | undefined,
  label: string,
  start: { jobId: string; startSeq: number },
): Promise<
  | { kind: 'background'; jobId: string }
  | { kind: 'foreground'; jobId: string; status: string; output: string; error: string | null }
> {
  if (background === true) {
    const jobs = ctx.get('jobs')
    if (jobs === undefined) {
      throw new Error('background jobs unavailable: load @deepseek-ai/dsh-jobs and @deepseek-ai/dsh-tool-jobs')
    }
    const id = jobs.start({
      kind: 'shellink',
      label,
      ...exec.agent ? { owner: exec.agent } : {},
      run: () => ctx.shellink.pollJob(start.jobId, start.startSeq),
    })
    return { kind: 'background', jobId: String(id) }
  }
  const result = await waitForDaemonJob(await ctx.shellink.ensureClient(), start.jobId, start.startSeq, exec.signal)
  return {
    kind: 'foreground',
    jobId: result.job.id,
    status: result.job.status,
    output: result.output,
    error: result.job.error,
  }
}

function jobStartOf(start: ExecStartResult | DaemonJobSnapshot): { jobId: string; startSeq: number } {
  if ('jobId' in start) return { jobId: start.jobId, startSeq: start.startSeq }
  return { jobId: start.id, startSeq: start.startSeq }
}

/** Register the Shellink tool suite. */
export function apply(ctx: Context): void {
  const service: ShellinkService = ctx.shellink
  const gate = new ApprovalGate(ctx, service)
  ctx.systemPrompt.section({
    name: 'tool:shellink',
    order: 120,
    text: SHELLINK_PROMPT,
  })

  ctx.tools.register(defineTool({
    name: 'shellink_profile_list',
    description: 'Search Shellink connection profiles by name, host, or command keyword. Do not list every profile; pass a query.',
    parameters: {
      query: { type: 'string', description: 'Case-insensitive keyword matched against profile name, host, and command.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { profiles: { type: 'array', required: true, items: OPAQUE_OBJECT } } },
      render: (_args, value) => [{ type: 'text', text: json(value.profiles) }],
    },
    isConcurrencySafe: () => true,
    async execute(args) {
      try {
        return { profiles: await service.listProfiles(args.query) }
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'List Shellink profiles', kind: 'read', rawInput: args.query ?? '' }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_profile_get',
    description: 'Get one Shellink profile by id. Credentials are never returned — only hasPassword / hasPrivateKey flags.',
    parameters: {
      profile_id: { type: 'string', required: true, description: 'Profile id from shellink_profile_list.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { profile: REQUIRED_OPAQUE_OBJECT } },
      render: (_args, value) => [{ type: 'text', text: json(value.profile) }],
    },
    isConcurrencySafe: () => true,
    async execute(args) {
      try {
        return { profile: await service.getProfile(args.profile_id) }
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Get Shellink profile', kind: 'read', rawInput: args.profile_id }),
  }))

  if (service.settings.profileWrites) {
    ctx.tools.register(defineTool({
      name: 'shellink_profile_create',
      description: 'Create a Shellink profile. Pass password_ref / private_key_ref / passphrase_ref — never plaintext secrets. Those refs must already be stored in the harness credential store or environment.',
      parameters: {
        name: { type: 'string', required: true, description: 'Display name.' },
        connect_type: { type: 'string', enum: ['ssh', 'command'], description: 'ssh (default) or command (PTY command / expect script).' },
        command: { type: 'string', description: 'Command to run in a PTY when connect_type is command.' },
        host: { type: 'string', description: 'SSH host.' },
        port: { type: 'number', description: 'SSH port (default 22).' },
        username: { type: 'string', description: 'SSH username.' },
        auth_type: { type: 'string', enum: ['password', 'key'], description: 'SSH auth method.' },
        password_ref: { type: 'string', description: 'Credential reference name for the SSH password.' },
        private_key_ref: { type: 'string', description: 'Credential reference name for the SSH private key.' },
        passphrase_ref: { type: 'string', description: 'Credential reference name for the key passphrase.' },
        term: { type: 'string', description: 'TERM value.' },
        cols: { type: 'number', description: 'Terminal columns.' },
        rows: { type: 'number', description: 'Terminal rows.' },
        prompt_regex: { type: 'string', description: 'Optional prompt regex used to detect WAITING_INPUT.' },
        unique_id: { type: 'string', description: 'Optional business unique key (host/IP or external Guid).' },
      },
      output: {
        schema: { type: 'object', additionalProperties: false, properties: { profile: REQUIRED_OPAQUE_OBJECT } },
        render: (_args, value) => [{ type: 'text', text: json(value.profile) }],
      },
      async execute(args, exec) {
        try {
          await gate.profileWrite(exec, {
            name: args.name,
            ...args.connect_type ? { connectType: args.connect_type } : {},
            ...args.command !== undefined ? { command: args.command } : {},
            ...args.host !== undefined ? { host: args.host } : {},
            ...args.unique_id !== undefined ? { uniqueId: args.unique_id } : {},
          }, 'create')
          return {
            profile: await service.createProfile({
              name: args.name,
              ...args.connect_type ? { connectType: args.connect_type } : {},
              ...args.command !== undefined ? { command: args.command } : {},
              ...args.host !== undefined ? { host: args.host } : {},
              ...args.port !== undefined ? { port: args.port } : {},
              ...args.username !== undefined ? { username: args.username } : {},
              ...args.auth_type ? { authType: args.auth_type } : {},
              ...args.password_ref ? { passwordRef: args.password_ref } : {},
              ...args.private_key_ref ? { privateKeyRef: args.private_key_ref } : {},
              ...args.passphrase_ref ? { passphraseRef: args.passphrase_ref } : {},
              ...args.term !== undefined ? { term: args.term } : {},
              ...args.cols !== undefined ? { cols: args.cols } : {},
              ...args.rows !== undefined ? { rows: args.rows } : {},
              ...args.prompt_regex !== undefined ? { promptRegex: args.prompt_regex } : {},
              ...args.unique_id !== undefined ? { uniqueId: args.unique_id } : {},
            }),
          }
        } catch (error) { asError(error) }
      },
      presentCall: args => ({ card: 'generic', title: 'Create Shellink profile', kind: 'other', rawInput: args.name }),
    }))

    ctx.tools.register(defineTool({
      name: 'shellink_profile_update',
      description: 'Update a Shellink profile. Credential fields are refs only, never plaintext.',
      parameters: {
        profile_id: { type: 'string', required: true, description: 'Profile id to update.' },
        name: { type: 'string', description: 'Display name.' },
        connect_type: { type: 'string', enum: ['ssh', 'command'] },
        command: { type: 'string' },
        host: { type: 'string' },
        port: { type: 'number' },
        username: { type: 'string' },
        auth_type: { type: 'string', enum: ['password', 'key'] },
        password_ref: { type: 'string' },
        private_key_ref: { type: 'string' },
        passphrase_ref: { type: 'string' },
        term: { type: 'string' },
        cols: { type: 'number' },
        rows: { type: 'number' },
        prompt_regex: { type: 'string' },
        unique_id: { type: 'string' },
      },
      output: {
        schema: { type: 'object', additionalProperties: false, properties: { profile: REQUIRED_OPAQUE_OBJECT } },
        render: (_args, value) => [{ type: 'text', text: json(value.profile) }],
      },
      async execute(args, exec) {
        try {
          await gate.profileWrite(exec, {
            ...args.name !== undefined ? { name: args.name } : {},
            ...args.connect_type ? { connectType: args.connect_type } : {},
            ...args.command !== undefined ? { command: args.command } : {},
            ...args.host !== undefined ? { host: args.host } : {},
            ...args.unique_id !== undefined ? { uniqueId: args.unique_id } : {},
          }, 'update')
          return {
            profile: await service.updateProfile(args.profile_id, {
              ...args.name !== undefined ? { name: args.name } : {},
              ...args.connect_type ? { connectType: args.connect_type } : {},
              ...args.command !== undefined ? { command: args.command } : {},
              ...args.host !== undefined ? { host: args.host } : {},
              ...args.port !== undefined ? { port: args.port } : {},
              ...args.username !== undefined ? { username: args.username } : {},
              ...args.auth_type ? { authType: args.auth_type } : {},
              ...args.password_ref ? { passwordRef: args.password_ref } : {},
              ...args.private_key_ref ? { privateKeyRef: args.private_key_ref } : {},
              ...args.passphrase_ref ? { passphraseRef: args.passphrase_ref } : {},
              ...args.term !== undefined ? { term: args.term } : {},
              ...args.cols !== undefined ? { cols: args.cols } : {},
              ...args.rows !== undefined ? { rows: args.rows } : {},
              ...args.prompt_regex !== undefined ? { promptRegex: args.prompt_regex } : {},
              ...args.unique_id !== undefined ? { uniqueId: args.unique_id } : {},
            }),
          }
        } catch (error) { asError(error) }
      },
      presentCall: args => ({ card: 'generic', title: 'Update Shellink profile', kind: 'other', rawInput: args.profile_id }),
    }))

    ctx.tools.register(defineTool({
      name: 'shellink_profile_delete',
      description: 'Delete a Shellink profile. Does not close sessions that already use it.',
      parameters: {
        profile_id: { type: 'string', required: true, description: 'Profile id to delete.' },
      },
      output: {
        schema: { type: 'object', additionalProperties: false, properties: { ok: { type: 'boolean', required: true }, profile_id: { type: 'string', required: true } } },
        render: (_args, value) => [{ type: 'text', text: `Deleted profile ${value.profile_id}` }],
      },
      async execute(args, exec) {
        try {
          if (service.settings.approval.gateProfileDelete) {
            await gate.plain(exec, 'shellink_profile_delete', `delete profile ${args.profile_id}`)
          }
          await service.deleteProfile(args.profile_id)
          return { ok: true, profile_id: args.profile_id }
        } catch (error) { asError(error) }
      },
      presentCall: args => ({ card: 'generic', title: 'Delete Shellink profile', kind: 'delete', rawInput: args.profile_id }),
    }))
  }

  ctx.tools.register(defineTool({
    name: 'shellink_session_list',
    description: 'List Shellink sessions known to the local daemon, including inactive records.',
    parameters: {},
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { sessions: { type: 'array', required: true, items: OPAQUE_OBJECT } } },
      render: (_args, value) => [{ type: 'text', text: json(value.sessions) }],
    },
    isConcurrencySafe: () => true,
    async execute() {
      try {
        return { sessions: await service.listSessions() }
      } catch (error) { asError(error) }
    },
    presentCall: () => ({ card: 'generic', title: 'List Shellink sessions', kind: 'read', rawInput: '' }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_open',
    description: 'Create a session from a profile and wait until it is AUTO and WAITING_INPUT.',
    parameters: {
      profile_id: { type: 'string', required: true, description: 'Profile id to connect.' },
      cols: { type: 'number', description: 'Terminal columns.' },
      rows: { type: 'number', description: 'Terminal rows.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { session: REQUIRED_OPAQUE_OBJECT } },
      render: (_args, value) => [{ type: 'text', text: json(value.session) }],
    },
    async execute(args, exec) {
      try {
        await gate.connect(exec, args.profile_id)
        return { session: await service.openSession(args.profile_id, args.cols, args.rows) }
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Open Shellink session', kind: 'execute', rawInput: args.profile_id }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_state',
    description: 'Read the current state and mode of a Shellink session.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { state: REQUIRED_OPAQUE_OBJECT } },
      render: (_args, value) => [{ type: 'text', text: json(value.state) }],
    },
    isConcurrencySafe: () => true,
    async execute(args) {
      try {
        return { state: await service.sessionState(args.session_id) }
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink session state', kind: 'read', rawInput: args.session_id }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_history',
    description: 'Read incremental session output from a cursor (history seq).',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      since: { type: 'number', description: 'History cursor (seq). Default 0.' },
      limit: { type: 'number', description: 'Max chunks to return (default 2000).' },
    },
    output: {
      schema: {
        type: 'object',
        additionalProperties: false,
        properties: {
          cursor: { type: 'number', required: true },
          text: { type: 'string', required: true },
        },
      },
      render: (_args, value) => [{ type: 'text', text: `cursor=${value.cursor}\n${value.text}` }],
    },
    isConcurrencySafe: () => true,
    async execute(args) {
      try {
        return await service.sessionHistory(args.session_id, args.since, args.limit)
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink session history', kind: 'read', rawInput: args.session_id }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_input',
    description: 'Send text to a session stdin (for answering a prompt). Prefer exec for complete commands.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      text: { type: 'string', required: true, description: 'Text to write.' },
      append_newline: { type: 'boolean', description: 'Append a newline (default true).' },
    },
    output: {
      schema: { type: 'object', additionalProperties: true, properties: { ok: { type: 'boolean', required: true } } },
      render: (_args, value) => [{ type: 'text', text: json(value) }],
    },
    async execute(args, exec) {
      try {
        await gate.input(exec, args.session_id, args.text)
        return await service.sessionInput(args.session_id, args.text, args.append_newline ?? true)
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink session input', kind: 'execute', rawInput: args.text }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_exec',
    description: 'Run a command on a Shellink session. The session must be AUTO and WAITING_INPUT. Use run_in_background for long commands and collect with job_output.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      command: { type: 'string', required: true, description: 'Remote command to run.' },
      timeout_ms: { type: 'number', description: 'Daemon exec timeout in milliseconds (default 20000).' },
      run_in_background: { type: 'boolean', description: 'Return a job id immediately; collect with job_output, stop with job_kill.' },
    },
    output: {
      schema: {
        oneOf: [
          { type: 'object', additionalProperties: false, properties: BACKGROUND_OUTPUT },
          { type: 'object', additionalProperties: false, properties: FOREGROUND_JOB_OUTPUT },
        ],
      },
      render: (_args, value) => [{
        type: 'text',
        text: value.kind === 'background'
          ? `started background job ${value.jobId}`
          : `status=${value.status}\n${value.output}${value.error ? `\nerror: ${value.error}` : ''}`,
      }],
    },
    async execute(args, exec) {
      try {
        await gate.command(exec, args.session_id, args.command, 'shellink_session_exec')
        const start = await service.execStart(args.session_id, args.command, args.timeout_ms)
        return await runStartedJob(ctx, exec, args.run_in_background, args.command, jobStartOf(start))
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink exec', kind: 'execute', rawInput: args.command }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_mode',
    description: 'Switch a session between AUTO (agent) and MANUAL (human takeover).',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      mode: { type: 'string', required: true, enum: ['AUTO', 'MANUAL'], description: 'AUTO or MANUAL.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: false, properties: { ok: { type: 'boolean', required: true }, mode: { type: 'string', required: true } } },
      render: (_args, value) => [{ type: 'text', text: `mode=${value.mode}` }],
    },
    async execute(args, exec) {
      try {
        if (args.mode === 'AUTO') await gate.modeAuto(exec, args.session_id)
        return await service.sessionMode(args.session_id, args.mode)
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Set Shellink session mode', kind: 'other', rawInput: args.mode }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_close',
    description: 'Close a live Shellink session. Keep sessions open for reuse unless the user asks to close.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
    },
    output: {
      schema: { type: 'object', additionalProperties: true, properties: { ok: { type: 'boolean', required: true } } },
      render: (_args, value) => [{ type: 'text', text: json(value) }],
    },
    async execute(args, exec) {
      try {
        if (service.settings.approval.gateSessionClose) {
          await gate.plain(exec, 'shellink_session_close', `close session ${args.session_id}`)
        }
        return await service.sessionClose(args.session_id)
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Close Shellink session', kind: 'delete', rawInput: args.session_id }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_upload',
    description: 'Upload a local file to a remote path over the session PTY. The local path is read through the harness filesystem policy.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      local_path: { type: 'string', required: true, description: 'Local file path in the current workspace.' },
      remote_path: { type: 'string', required: true, description: 'Remote destination path.' },
      run_in_background: { type: 'boolean', description: 'Return a job id immediately.' },
    },
    output: {
      schema: {
        oneOf: [
          { type: 'object', additionalProperties: false, properties: BACKGROUND_OUTPUT },
          { type: 'object', additionalProperties: false, properties: FOREGROUND_JOB_OUTPUT },
        ],
      },
      render: (_args, value) => [{
        type: 'text',
        text: value.kind === 'background' ? `started background job ${value.jobId}` : `status=${value.status}\n${value.output}`,
      }],
    },
    async execute(args, exec) {
      try {
        await gate.remoteWrite(exec, args.session_id, args.remote_path, 'upload')
        const target = await ctx.fs.resolve(args.local_path, { signal: exec.signal })
        const info = await ctx.fs.stat(target, exec.signal)
        const max = service.settings.uploadMaxBytes
        if (info?.size !== undefined && info.size > max) {
          throw new Error(`file is ${info.size} bytes; uploadMaxBytes is ${max}`)
        }
        const data = await ctx.fs.readBytes(target, exec.signal, max)
        if (data.byteLength > max) throw new Error(`file is ${data.byteLength} bytes; uploadMaxBytes is ${max}`)
        const start = await service.uploadStart(args.session_id, args.remote_path, data)
        return await runStartedJob(ctx, exec, args.run_in_background, `upload ${args.remote_path}`, jobStartOf(start))
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink upload', kind: 'execute', rawInput: args.remote_path }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_download',
    description: 'Download a remote file to a local path. The plugin checks the local path against the harness filesystem policy; the Shellink daemon writes the bytes.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      remote_path: { type: 'string', required: true, description: 'Remote source path.' },
      local_path: { type: 'string', required: true, description: 'Local destination path.' },
      run_in_background: { type: 'boolean', description: 'Return a job id immediately.' },
    },
    output: {
      schema: {
        oneOf: [
          { type: 'object', additionalProperties: false, properties: BACKGROUND_OUTPUT },
          { type: 'object', additionalProperties: false, properties: FOREGROUND_JOB_OUTPUT },
        ],
      },
      render: (_args, value) => [{
        type: 'text',
        text: value.kind === 'background' ? `started background job ${value.jobId}` : `status=${value.status}\n${value.output}`,
      }],
    },
    async execute(args, exec) {
      try {
        await gate.remoteRead(exec, args.session_id, args.remote_path)
        const target = await ctx.fs.resolve(args.local_path, { signal: exec.signal })
        const output = ctx.fs.processPath(target)
        const start = await service.downloadStart(args.session_id, args.remote_path, output)
        return await runStartedJob(ctx, exec, args.run_in_background, `download ${args.remote_path}`, jobStartOf(start))
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink download', kind: 'execute', rawInput: args.remote_path }),
  }))

  ctx.tools.register(defineTool({
    name: 'shellink_session_edit',
    description: 'Apply exact, non-overlapping replacements on a remote file. Requires AUTO + WAITING_INPUT. Each old_text must match exactly once.',
    parameters: {
      session_id: { type: 'string', required: true, description: 'Session id.' },
      path: { type: 'string', required: true, description: 'Remote file path.' },
      edits: {
        type: 'array',
        required: true,
        description: 'Exact replacements. old_text must be unique in the file; edits must not overlap.',
        items: {
          type: 'object',
          additionalProperties: false,
          properties: {
            old_text: { type: 'string', required: true, description: 'Exact text to replace.' },
            new_text: { type: 'string', required: true, description: 'Replacement text.' },
          },
        },
      },
      run_in_background: { type: 'boolean', description: 'Return a job id immediately.' },
    },
    output: {
      schema: {
        oneOf: [
          { type: 'object', additionalProperties: false, properties: BACKGROUND_OUTPUT },
          { type: 'object', additionalProperties: false, properties: FOREGROUND_JOB_OUTPUT },
        ],
      },
      render: (_args, value) => [{
        type: 'text',
        text: value.kind === 'background' ? `started background job ${value.jobId}` : `status=${value.status}\n${value.output}`,
      }],
    },
    async execute(args, exec) {
      try {
        if (args.edits.length === 0) throw new Error('edits must contain at least one replacement')
        await gate.remoteWrite(exec, args.session_id, args.path, 'edit')
        const start = await service.editStart(
          args.session_id,
          args.path,
          args.edits.map(edit => ({ oldText: edit.old_text, newText: edit.new_text })),
        )
        return await runStartedJob(ctx, exec, args.run_in_background, `edit ${args.path}`, jobStartOf(start))
      } catch (error) { asError(error) }
    },
    presentCall: args => ({ card: 'generic', title: 'Shellink edit', kind: 'execute', rawInput: args.path }),
  }))
}
