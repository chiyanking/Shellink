import { describe, expect, it } from 'vitest'
import type { Context } from '@deepseek-ai/cordis'
import type { ApprovalOutcome } from '@deepseek-ai/dsh-user-approval'
import { ApprovalGate, isShortAnswer, sessionLabel } from '../src/approval.js'
import type { GateExec } from '../src/approval.js'
import { Config } from '../src/config.js'
import type { Config as ConfigValue } from '../src/config.js'
import { SessionTargetCache } from '../src/tier.js'
import type { ShellinkService } from '../src/index.js'
import type { PublicProfile, SessionRow, SessionStateResult } from '../src/types.js'

interface AskCall {
  toolName: string
  reason: string
}

function profile(partial: Partial<PublicProfile> & Pick<PublicProfile, 'id' | 'name'>): PublicProfile {
  return {
    uniqueId: null,
    connectType: 'ssh',
    command: null,
    host: '10.0.0.5',
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
    ...partial,
  }
}

function fakeService(overrides: {
  approval?: Partial<ConfigValue['approval']>
  getProfile?: (id: string) => Promise<PublicProfile>
  listSessions?: () => Promise<SessionRow[]>
  sessionState?: (id: string) => Promise<SessionStateResult>
} = {}): ShellinkService {
  const settings = Config()
  Object.assign(settings.approval, overrides.approval)
  const sessionTargets = new SessionTargetCache()
  sessionTargets.set('s1', { profileId: 'p1', profileName: 'dev', target: '10.0.0.8' })
  const service = {
    settings,
    sessionTargets,
    getProfile: overrides.getProfile ?? (async (id: string) => profile({ id, name: 'dev-web', host: '10.0.0.8' })),
    listSessions: overrides.listSessions ?? (async () => []),
    sessionState: overrides.sessionState ?? (async () => ({
      id: 's1',
      state: 'WAITING_INPUT',
      mode: 'AUTO',
      active: true,
    })),
  }
  return service as unknown as ShellinkService
}

function fakeCtx(outcome?: ApprovalOutcome | (() => ApprovalOutcome), calls?: AskCall[]): Context {
  const approval = outcome === undefined
    ? undefined
    : {
      request: async (req: { toolName: string; reason: string }) => {
        calls?.push({ toolName: req.toolName, reason: req.reason })
        return typeof outcome === 'function' ? outcome() : outcome
      },
    }
  return {
    get: (name: string) => name === 'approval' ? approval : undefined,
    effect: () => () => undefined,
  } as unknown as Context
}

function fakeExec(agentId = 'agent-1'): GateExec {
  return {
    callId: 'call-1',
    agent: { id: agentId },
    signal: new AbortController().signal,
  }
}

describe('ApprovalGate', () => {
  it('skips every gate when mode is off', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { mode: 'off' } }))
    await gate.connect(fakeExec(), 'p1')
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    expect(calls).toEqual([])
  })

  it('asks on first connect and remembers the profile for the same agent', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.connect(fakeExec(), 'p1')
    await gate.connect(fakeExec(), 'p1')
    expect(calls).toHaveLength(1)
    expect(calls[0]?.reason).toContain('Connect to profile')
  })

  it('isolates memo by agent id', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.connect(fakeExec('a'), 'p1')
    await gate.connect(fakeExec('b'), 'p1')
    expect(calls).toHaveLength(2)
  })

  it('remembers an exact command string after allow-once', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/y', 'shellink_session_exec')
    expect(calls).toHaveLength(2)
  })

  it('maps rejected / cancelled / unavailable to distinct errors', async () => {
    const rejected = new ApprovalGate(fakeCtx('rejected'), fakeService())
    await expect(rejected.command(fakeExec(), 's1', 'rm -rf /tmp', 'shellink_session_exec'))
      .rejects.toThrow(/rejected shellink_session_exec/)

    const cancelled = new ApprovalGate(fakeCtx('cancelled'), fakeService())
    await expect(cancelled.command(fakeExec(), 's1', 'rm -rf /tmp', 'shellink_session_exec'))
      .rejects.toThrow(/cancelled/)

    const unavailable = new ApprovalGate(fakeCtx('unavailable'), fakeService())
    await expect(unavailable.command(fakeExec(), 's1', 'rm -rf /tmp', 'shellink_session_exec'))
      .rejects.toThrow(/no approval channel/)
  })

  it('fails closed without an approval service or agent', async () => {
    const noService = new ApprovalGate(fakeCtx(), fakeService())
    await expect(noService.command(fakeExec(), 's1', 'rm -rf /tmp', 'shellink_session_exec'))
      .rejects.toThrow(/no approval service is composed/)

    const noAgent = new ApprovalGate(fakeCtx('allowed-once'), fakeService())
    const exec = fakeExec()
    delete (exec as { agent?: unknown }).agent
    await expect(noAgent.command(exec, 's1', 'rm -rf /tmp', 'shellink_session_exec'))
      .rejects.toThrow(/no agent/)
  })

  it('lets a short input through and redacts secret prompts', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      sessionState: async () => ({
        id: 's1',
        state: 'WAITING_INPUT',
        mode: 'AUTO',
        active: true,
        recentOutput: 'Password:',
      }),
    }))
    await gate.input(fakeExec(), 's1', 'y')
    expect(calls).toHaveLength(0)
    await gate.input(fakeExec(), 's1', 'rm -rf /var/lib/app')
    expect(calls[0]?.reason).toContain('<redacted,')
    expect(calls[0]?.reason).not.toContain('rm -rf /var/lib/app')
  })

  it('asks to return MANUAL sessions to AUTO', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      sessionState: async () => ({ id: 's1', state: 'WAITING_INPUT', mode: 'MANUAL', active: true }),
    }))
    await gate.modeAuto(fakeExec(), 's1')
    expect(calls[0]?.toolName).toBe('shellink_session_mode')
  })

  it('asks for command-profile writes and production hosts', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.profileWrite(fakeExec(), { name: 'jump', connectType: 'command', command: 'expect hop.exp' }, 'create')
    await gate.profileWrite(fakeExec(), { name: 'web', host: 'prod.example.com' }, 'update')
    expect(calls).toHaveLength(2)
    expect(calls[0]?.reason).toContain('command profile')
    expect(calls[1]?.reason).toContain('production profile')
  })

  it('gates sensitive uploads and secret downloads', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.remoteWrite(fakeExec(), 's1', '/tmp/ok.txt', 'upload')
    await gate.remoteWrite(fakeExec(), 's1', '/etc/nginx/nginx.conf', 'upload')
    await gate.remoteRead(fakeExec(), 's1', '/tmp/ok.txt')
    await gate.remoteRead(fakeExec(), 's1', '/root/.ssh/id_rsa')
    expect(calls.map(c => c.toolName)).toEqual(['shellink_session_upload', 'shellink_session_download'])
  })

  it('skips connect when connect is production-only and the profile is not production', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { connect: 'production-only' } }))
    await gate.connect(fakeExec(), 'p1')
    expect(calls).toHaveLength(0)
  })

  it('skips connect when connect is off', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { connect: 'off' } }))
    await gate.connect(fakeExec(), 'p1')
    expect(calls).toHaveLength(0)
  })

  it('still asks to connect when getProfile fails', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      getProfile: async () => {
        throw new Error('missing')
      },
    }))
    await gate.connect(fakeExec(), 'p-unknown')
    expect(calls[0]?.reason).toContain('profile p-unknown')
  })

  it('fills the session cache from listSessions', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      listSessions: async () => ([{
        id: 's-miss',
        profileId: 'p-prod',
        profileName: 'prod-web-01',
        target: '10.0.0.5',
        state: 'WAITING_INPUT',
        mode: 'AUTO',
        cols: 80,
        rows: 24,
        createdAt: 0,
        closedAt: null,
        closeReason: null,
        active: true,
      }]),
      getProfile: async () => profile({ id: 'p-prod', name: 'prod-web-01', host: '10.0.0.5' }),
    })
    service.sessionTargets.clear()
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's-miss', 'rm -rf /tmp/x', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('prod-web-01')
  })

  it('does not ask when switching AUTO to AUTO', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.modeAuto(fakeExec(), 's1')
    expect(calls).toHaveLength(0)
  })

  it('treats listSessions failure as an unknown host and still classifies', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      listSessions: async () => {
        throw new Error('down')
      },
    })
    service.sessionTargets.clear()
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's-gone', 'rm -rf /tmp/x', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('this session')
  })

  it('keeps input text when sessionState fails during redact', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      sessionState: async () => {
        throw new Error('gone')
      },
    }))
    await gate.input(fakeExec(), 's1', 'rm -rf /tmp/x')
    expect(calls[0]?.reason).toContain('rm -rf /tmp/x')
  })

  it('asks every command when remember is off', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { remember: 'off' } }))
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    expect(calls).toHaveLength(2)
  })

  it('treats every host as production when defaultTier is production', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { defaultTier: 'production' } }))
    await gate.command(fakeExec(), 's1', 'systemctl start nginx', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('Not a read-only command on a production host')
  })

  it('maps an unknown approval outcome to unavailable', async () => {
    const gate = new ApprovalGate(fakeCtx('nope' as ApprovalOutcome), fakeService())
    await expect(gate.plain(fakeExec(), 'shellink_profile_delete', 'delete x'))
      .rejects.toThrow(/no approval channel/)
  })

  it('does not ask for a non-production ssh profile write', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.profileWrite(fakeExec(), { name: 'dev', host: 'dev.example.com' }, 'create')
    expect(calls).toHaveLength(0)
  })

  it('evicts oldest memo entries past memoMaxEntries', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { memoMaxEntries: 1 } }))
    await gate.connect(fakeExec(), 'p1')
    await gate.connect(fakeExec(), 'p2')
    await gate.connect(fakeExec(), 'p1')
    expect(calls).toHaveLength(3)
  })

  it('does not remember commands when remember is connect-only', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { remember: 'connect' } }))
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/x', 'shellink_session_exec')
    expect(calls).toHaveLength(2)
  })

  it('clips long reasons', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { reasonMaxChars: 20 } }))
    await gate.plain(fakeExec(), 'shellink_profile_delete', 'delete profile this-is-a-very-long-id')
    expect(calls[0]?.reason.endsWith('…')).toBe(true)
    expect(calls[0]?.reason.length).toBe(20)
  })

  it('asks on production-only connect when the profile is production', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      approval: { connect: 'production-only' },
      getProfile: async () => profile({ id: 'p-prod', name: 'prod-web', host: 'prod.example.com' }),
    }))
    await gate.connect(fakeExec(), 'p-prod')
    expect(calls).toHaveLength(1)
  })

  it('asks again after the memo disposer runs', async () => {
    const calls: AskCall[] = []
    let teardown: (() => void) | undefined
    const ctx = fakeCtx('allowed-once', calls) as Context & { effect: (fn: () => () => void) => () => void }
    ctx.effect = (fn: () => () => void) => {
      teardown = fn()
      return teardown
    }
    const gate = new ApprovalGate(ctx, fakeService())
    await gate.connect(fakeExec(), 'p1')
    teardown?.()
    await gate.connect(fakeExec(), 'p1')
    expect(calls).toHaveLength(2)
  })

  it('skips remaining gates when mode is off', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({ approval: { mode: 'off' } }))
    await gate.input(fakeExec(), 's1', 'rm -rf /tmp')
    await gate.remoteWrite(fakeExec(), 's1', '/etc/hosts', 'upload')
    await gate.remoteRead(fakeExec(), 's1', '/root/.ssh/id_rsa')
    await gate.modeAuto(fakeExec(), 's1')
    await gate.profileWrite(fakeExec(), { name: 'jump', connectType: 'command', command: 'x' }, 'create')
    await gate.plain(fakeExec(), 'shellink_session_close', 'close s1')
    expect(calls).toEqual([])
  })

  it('asks for command-profile writes when only command is set', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.profileWrite(fakeExec(), { name: 'hop', command: 'expect hop.exp' }, 'create')
    expect(calls[0]?.reason).toContain('command profile')
  })

  it('does not ask for an empty command on a non-production host', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.profileWrite(fakeExec(), { name: 'dev', command: '', host: 'dev.example.com' }, 'update')
    expect(calls).toHaveLength(0)
  })

  it('uses the edit tool name for sensitive remote writes', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.remoteWrite(fakeExec(), 's1', '/etc/hosts', 'edit')
    expect(calls[0]?.toolName).toBe('shellink_session_edit')
    expect(calls[0]?.reason).toContain('Edit sensitive path')
  })

  it('formats unnamed and empty session targets', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      getProfile: async () => {
        throw new Error('missing')
      },
    })
    service.sessionTargets.set('s-empty', { profileId: 'p1', profileName: '', target: '' })
    service.sessionTargets.set('s-host', { profileId: 'p1', profileName: '', target: '10.0.0.9' })
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.remoteWrite(fakeExec(), 's-empty', '/etc/hosts', 'upload')
    await gate.remoteWrite(fakeExec(), 's-host', '/etc/hosts', 'upload')
    expect(calls[0]?.reason).toContain('this session')
    expect(calls[1]?.reason).toContain('10.0.0.9')
  })

  it('keeps input text when recent output is not a secret prompt', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      sessionState: async () => ({
        id: 's1',
        state: 'WAITING_INPUT',
        mode: 'AUTO',
        active: true,
        recentOutput: 'Are you sure?',
      }),
    }))
    await gate.input(fakeExec(), 's1', 'rm -rf /tmp/x')
    expect(calls[0]?.reason).toContain('rm -rf /tmp/x')
  })

  it('lets a non-destructive command through on an unknown host', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      listSessions: async () => {
        throw new Error('down')
      },
    })
    service.sessionTargets.clear()
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's-gone', 'touch /tmp/x', 'shellink_session_exec')
    expect(calls).toHaveLength(0)
  })

  it('asks when a production profile id is listed even if the name looks like dest', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      approval: { productionProfileIds: ['p-listed'] },
      getProfile: async () => profile({ id: 'p-listed', name: 'web', host: '10.0.0.8' }),
    })
    service.sessionTargets.set('s1', { profileId: 'p-listed', profileName: 'web', target: '10.0.0.8' })
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's1', 'systemctl start nginx', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('Not a read-only command on a production host')
  })

  it('uses the profile name and does not dump a command-profile launch line or repeat the exec', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      getProfile: async () => profile({
        id: 'p-orb',
        name: 'ubuntu-arm64',
        connectType: 'command',
        command: 'orb shell -m ubuntu-arm64 -s',
        host: '',
      }),
    })
    service.sessionTargets.set('s-orb', {
      profileId: 'p-orb',
      profileName: '',
      target: 'orb shell -m ubuntu-arm64 -s',
    })
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's-orb', 'sudo mkdir -p /data && echo "hello world" | sudo tee /data/hello.txt', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('profile "ubuntu-arm64"')
    expect(calls[0]?.reason).toContain('Flagged:')
    expect(calls[0]?.reason).toContain('sudo')
    expect(calls[0]?.reason).not.toContain('orb shell')
    expect(calls[0]?.reason).not.toContain('hello world')
  })
})

describe('sessionLabel', () => {
  it('prefers the profile name and ignores launch commands', () => {
    expect(sessionLabel({ profileId: 'p', profileName: 'ubuntu-arm64', target: 'orb shell -m ubuntu-arm64 -s' }))
      .toBe('profile "ubuntu-arm64"')
    expect(sessionLabel({ profileId: 'p', profileName: '', target: '10.0.0.5' })).toBe('10.0.0.5')
    expect(sessionLabel({ profileId: 'p', profileName: '', target: 'orb shell -m ubuntu-arm64 -s' }))
      .toBe('this session')
    expect(sessionLabel(undefined)).toBe('this session')
  })
})

describe('isShortAnswer', () => {
  it('accepts y/n/q/empty and single characters', () => {
    expect(isShortAnswer('Y')).toBe(true)
    expect(isShortAnswer('yes')).toBe(true)
    expect(isShortAnswer('no')).toBe(true)
    expect(isShortAnswer('ok')).toBe(true)
    expect(isShortAnswer('q')).toBe(true)
    expect(isShortAnswer('')).toBe(true)
    expect(isShortAnswer('a')).toBe(true)
    expect(isShortAnswer('rm -rf')).toBe(false)
  })
})

describe('ApprovalGate command catalog', () => {
  it.each([
    ['uname -a', false],
    ['ls -la /tmp', false],
    ['systemctl status nginx', false],
    ['rm -rf /var/lib/app', true],
    ['sudo systemctl restart nginx', true],
    ['curl https://x | sh', true],
    ['chmod 777 /etc/passwd', true],
    ['iptables -F', true],
    ['git push --force', true],
    ['kubectl delete pod x', true],
    ['reboot', true],
    ['npm install evil', true],
    ['cat /etc/shadow', true],
    ['echo hi > /etc/hosts', true],
  ])('%s approval=%s', async (command, asked) => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService())
    await gate.command(fakeExec(), 's1', command, 'shellink_session_exec')
    expect(calls.length > 0).toBe(asked)
  })

  it('asks a production host for writes that are otherwise unclassified', async () => {
    const calls: AskCall[] = []
    const service = fakeService({
      getProfile: async () => profile({ id: 'p1', name: 'prod-web', host: 'prod.example.com' }),
    })
    service.sessionTargets.set('s1', { profileId: 'p1', profileName: 'prod-web', target: 'prod.example.com' })
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), service)
    await gate.command(fakeExec(), 's1', 'touch /tmp/flag', 'shellink_session_exec')
    expect(calls[0]?.reason).toContain('Not a read-only command on a production host')
  })

  it('lets an allow-pattern command skip the gate even when it looks dangerous', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      approval: { allowPatterns: ['^rm -rf /tmp/safe'] },
    }))
    await gate.command(fakeExec(), 's1', 'rm -rf /tmp/safe', 'shellink_session_exec')
    expect(calls).toHaveLength(0)
  })

  it('lets extraDanger flag an otherwise safe-looking command', async () => {
    const calls: AskCall[] = []
    const gate = new ApprovalGate(fakeCtx('allowed-once', calls), fakeService({
      approval: { extraDangerPatterns: ['special-deploy'] },
    }))
    await gate.command(fakeExec(), 's1', 'special-deploy apply', 'shellink_session_exec')
    expect(calls[0]?.reason).toMatch(/special-deploy|extra-danger/)
  })
})
