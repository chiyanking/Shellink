import { describe, expect, it } from 'vitest'
import type { Context } from '@deepseek-ai/cordis'
import type { ToolRunContext } from '@deepseek-ai/dsh-tools'
import { apply } from '../src/tools.js'
import { Config } from '../src/config.js'
import { SessionTargetCache } from '../src/tier.js'

interface RegisteredTool {
  name: string
  execute: (args: Record<string, unknown>, exec: ToolRunContext) => Promise<unknown>
}

function fakeExec(): ToolRunContext {
  return {
    callId: 'call-1',
    rootCallId: 'call-1',
    name: 'shellink_session_exec',
    arguments: {},
    agent: { id: 'agent-1' },
    signal: new AbortController().signal,
    deferContext: () => undefined,
    concludeTurn: () => undefined,
  } as unknown as ToolRunContext
}

function mockContext(profileWrites: boolean, extras?: {
  outcome?: 'allowed-once' | 'rejected'
  execStart?: () => Promise<unknown>
}): { ctx: Context; names: string[]; tools: RegisteredTool[] } {
  const names: string[] = []
  const tools: RegisteredTool[] = []
  const settings = Config()
  settings.profileWrites = profileWrites
  const approval = extras?.outcome === undefined
    ? undefined
    : { request: async () => extras.outcome }
  const ctx = {
    shellink: {
      settings,
      sessionTargets: new SessionTargetCache(),
      getProfile: async () => ({
        id: 'p1',
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
      }),
      listSessions: async () => [],
      execStart: extras?.execStart ?? (async () => {
        throw new Error('execStart should not run')
      }),
    },
    systemPrompt: { section: () => undefined },
    tools: {
      register: (tool: RegisteredTool) => {
        names.push(tool.name)
        tools.push(tool)
      },
    },
    get: (name: string) => name === 'approval' ? approval : undefined,
    effect: () => () => undefined,
  }
  return { ctx: ctx as unknown as Context, names, tools }
}

describe('tool registration', () => {
  it('registers the full suite when profileWrites is on', () => {
    const { ctx, names } = mockContext(true)
    apply(ctx)
    expect(names).toContain('shellink_profile_create')
    expect(names).toContain('shellink_session_exec')
    expect(names).toContain('shellink_session_upload')
    expect(names).toContain('shellink_session_download')
    expect(names).toContain('shellink_session_edit')
  })

  it('omits profile write tools when profileWrites is off', () => {
    const { ctx, names } = mockContext(false)
    apply(ctx)
    expect(names).not.toContain('shellink_profile_create')
    expect(names).toContain('shellink_profile_list')
    expect(names).toContain('shellink_session_open')
  })

  it('asks before a dangerous exec and does not start the job when rejected', async () => {
    let started = false
    const { ctx, tools } = mockContext(true, {
      outcome: 'rejected',
      execStart: async () => {
        started = true
        return { jobId: 'j1', startSeq: 1 }
      },
    })
    apply(ctx)
    const exec = tools.find(tool => tool.name === 'shellink_session_exec')
    expect(exec).toBeDefined()
    await expect(exec?.execute({ session_id: 's1', command: 'rm -rf /tmp/x' }, fakeExec()))
      .rejects.toThrow(/rejected/)
    expect(started).toBe(false)
  })
})
