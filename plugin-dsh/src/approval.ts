/**
 * Approval gate: ask `ctx.approval` before connect / dangerous exec / file ops.
 * Grants are one-shot; remembered decisions stay in-process per agent session.
 * @module shellink-dsh/approval
 */

import type { Context } from '@deepseek-ai/cordis'
import type { ApprovalOutcome } from '@deepseek-ai/dsh-user-approval'
import { classifyCommand, classifyRemotePath, normalizeCommand } from './danger.js'
import { compileApproval } from './config.js'
import type { CompiledApproval } from './config.js'
import { isProductionTarget } from './tier.js'
import type { SessionTarget } from './tier.js'
import type { ShellinkService } from './index.js'

const SHORT_ANSWERS = new Set(['', 'y', 'n', 'q', 'yes', 'no', 'ok'])
const SECRET_PROMPT = /password|passphrase|token|secret/i

export interface ProfileWriteGate {
  name?: string
  connectType?: string
  command?: string
  host?: string
  uniqueId?: string
}

/** Minimal tool-execution slice the gate needs; avoids a second ToolRunContext instance. */
export interface GateExec {
  readonly callId: string
  readonly agent?: { readonly id: unknown }
  readonly signal: AbortSignal
}

interface ApprovalAsker {
  request(req: {
    agent: NonNullable<GateExec['agent']>
    toolName: string
    callId: string
    reason: string
    signal?: AbortSignal
  }): Promise<ApprovalOutcome>
}

/** Ask the composed approval service before mutating remote sessions. */
export class ApprovalGate {
  private readonly compiled: CompiledApproval
  private readonly memo = new Map<string, true>()

  constructor(
    private readonly ctx: Context,
    private readonly service: ShellinkService,
  ) {
    this.compiled = compileApproval(service.settings.approval)
    ctx.effect(() => () => {
      this.memo.clear()
    }, 'shellink approval memo teardown')
  }

  async connect(exec: GateExec, profileId: string): Promise<void> {
    if (this.off() || this.service.settings.approval.connect === 'off') return
    const profile = await this.safeGetProfile(profileId)
    const production = isProductionTarget({
      id: profile?.id ?? profileId,
      name: profile?.name,
      host: profile?.host,
      uniqueId: profile?.uniqueId,
    }, this.service.settings.approval, this.compiled)
    if (this.service.settings.approval.connect === 'production-only' && !production) return
    const memoKey = this.profileMemoKey(exec, profileId)
    if (memoKey !== undefined && this.memoHit(memoKey)) return
    await this.ask(exec, 'shellink_session_open', `Connect to ${connectLabel(profile, profileId)}`, memoKey)
  }

  async command(exec: GateExec, sessionId: string, command: string, toolName: string): Promise<void> {
    await this.decideCommand(exec, sessionId, command, toolName)
  }

  async input(exec: GateExec, sessionId: string, text: string): Promise<void> {
    if (this.off() || isShortAnswer(text)) return
    await this.decideCommand(exec, sessionId, text, 'shellink_session_input', true)
  }

  async remoteWrite(exec: GateExec, sessionId: string, path: string, verb: string): Promise<void> {
    if (this.off()) return
    const kind = classifyRemotePath(path)
    if (kind === 'ok') return
    const where = sessionLabel(await this.sessionTarget(sessionId))
    await this.ask(exec, verb === 'upload' ? 'shellink_session_upload' : 'shellink_session_edit', this.clip(`${capitalize(verb)} ${kind} path on ${where}:\n${path}`))
  }

  async remoteRead(exec: GateExec, sessionId: string, path: string): Promise<void> {
    if (this.off()) return
    if (classifyRemotePath(path) !== 'secret') return
    const where = sessionLabel(await this.sessionTarget(sessionId))
    await this.ask(exec, 'shellink_session_download', this.clip(`Download secret path on ${where}:\n${path}`))
  }

  async modeAuto(exec: GateExec, sessionId: string): Promise<void> {
    if (this.off()) return
    const state = await this.service.sessionState(sessionId)
    if (state.mode !== 'MANUAL') return
    await this.ask(exec, 'shellink_session_mode', `return session ${sessionId} from MANUAL to AUTO`)
  }

  async profileWrite(exec: GateExec, input: ProfileWriteGate, verb: string): Promise<void> {
    if (this.off()) return
    const toolName = verb === 'create' ? 'shellink_profile_create' : 'shellink_profile_update'
    if (input.connectType === 'command' || (input.command !== undefined && input.command.length > 0)) {
      await this.ask(exec, toolName, this.clip(`${capitalize(verb)} command profile "${input.name ?? '?'}"\nLaunch: ${input.command ?? ''}`))
      return
    }
    if (input.host !== undefined && isProductionTarget({
      name: input.name,
      host: input.host,
      uniqueId: input.uniqueId,
    }, this.service.settings.approval, this.compiled)) {
      await this.ask(exec, toolName, this.clip(`${capitalize(verb)} production profile "${input.name ?? input.host}" (${input.host})`))
    }
  }

  async plain(exec: GateExec, toolName: string, reason: string): Promise<void> {
    if (this.off()) return
    await this.ask(exec, toolName, this.clip(reason))
  }

  private async decideCommand(
    exec: GateExec,
    sessionId: string,
    command: string,
    toolName: string,
    redact = false,
  ): Promise<void> {
    if (this.off()) return
    const normalized = normalizeCommand(command)
    const memoKey = this.commandMemoKey(exec, sessionId, normalized)
    if (memoKey !== undefined && this.memoHit(memoKey)) return
    const target = await this.sessionTarget(sessionId)
    const production = await this.sessionIsProduction(sessionId, target)
    const classified = classifyCommand(command, {
      extraDanger: this.compiled.extraDanger,
      allow: this.compiled.allow,
      readOnlyExtra: this.service.settings.approval.readOnlyExtra,
      production,
    })
    if (classified.risk === 'safe') return
    const where = sessionLabel(target)
    const flagged = formatFlagged(classified.rules)
    if (redact) {
      const shown = await this.redactIfSecret(sessionId, normalized)
      await this.ask(exec, toolName, this.clip(`Send input on ${where}:\n${shown}${flagged}`), memoKey)
      return
    }
    // The DSH card already prints args.command under the headline — do not repeat it.
    await this.ask(exec, toolName, this.clip(`Run on ${where}${flagged}`), memoKey)
  }

  private off(): boolean {
    return this.service.settings.approval.mode === 'off'
  }

  private async ask(
    exec: GateExec,
    toolName: string,
    reason: string,
    memoKey?: string,
  ): Promise<void> {
    const approval = this.ctx.get('approval') as ApprovalAsker | undefined
    if (approval === undefined) {
      throw new Error(`${toolName} requires approval, but no approval service is composed`)
    }
    if (exec.agent === undefined) {
      throw new Error(`${toolName} requires approval, but the call has no agent to route it through`)
    }
    const outcome = await approval.request({
      agent: exec.agent,
      toolName,
      callId: exec.callId,
      reason,
      signal: exec.signal,
    })
    switch (outcome) {
      case 'allowed-once':
        if (memoKey !== undefined) this.memoAdd(memoKey)
        return
      case 'rejected':
        throw new Error(`the user rejected ${toolName}`)
      case 'cancelled':
        throw new Error(`approval for ${toolName} was cancelled`)
      case 'unavailable':
        throw new Error(`${toolName} requires approval, but no approval channel is available`)
      default:
        throw new Error(`${toolName} requires approval, but no approval channel is available`)
    }
  }

  private profileMemoKey(exec: GateExec, profileId: string): string | undefined {
    const remember = this.service.settings.approval.remember
    if (remember === 'off' || exec.agent === undefined) return undefined
    return `p\0${String(exec.agent.id)}\0${profileId}`
  }

  private commandMemoKey(exec: GateExec, sessionId: string, command: string): string | undefined {
    if (this.service.settings.approval.remember !== 'connect+command') return undefined
    if (exec.agent === undefined) return undefined
    return `c\0${String(exec.agent.id)}\0${sessionId}\0${command}`
  }

  private memoHit(key: string): boolean {
    if (!this.memo.has(key)) return false
    this.memo.delete(key)
    this.memo.set(key, true)
    return true
  }

  private memoAdd(key: string): void {
    if (this.memo.has(key)) this.memo.delete(key)
    this.memo.set(key, true)
    const max = this.service.settings.approval.memoMaxEntries
    while (this.memo.size > max) {
      const oldest = this.memo.keys().next().value
      if (oldest === undefined) break
      this.memo.delete(oldest)
    }
  }

  private async safeGetProfile(profileId: string) {
    try {
      return await this.service.getProfile(profileId)
    } catch {
      return undefined
    }
  }

  private async sessionTarget(sessionId: string): Promise<SessionTarget | undefined> {
    let hit = this.service.sessionTargets.get(sessionId)
    if (hit === undefined) {
      try {
        const rows = await this.service.listSessions()
        this.service.sessionTargets.ingest(rows)
        hit = this.service.sessionTargets.get(sessionId)
      } catch {
        return undefined
      }
    }
    if (hit === undefined) return undefined
    if (hit.profileName.length > 0) return hit
    const profile = await this.safeGetProfile(hit.profileId)
    if (profile === undefined) return hit
    const enriched: SessionTarget = {
      profileId: hit.profileId,
      profileName: profile.name,
      target: profile.host || hit.target,
    }
    this.service.sessionTargets.set(sessionId, enriched)
    return enriched
  }

  private async sessionIsProduction(sessionId: string, target: SessionTarget | undefined): Promise<boolean> {
    if (this.service.settings.approval.defaultTier === 'production') return true
    if (target === undefined) return false
    const profile = await this.safeGetProfile(target.profileId)
    return isProductionTarget({
      id: profile?.id ?? target.profileId,
      name: profile?.name ?? target.profileName,
      host: profile?.host,
      target: target.target,
      uniqueId: profile?.uniqueId,
    }, this.service.settings.approval, this.compiled)
  }

  private async redactIfSecret(sessionId: string, text: string): Promise<string> {
    try {
      const state = await this.service.sessionState(sessionId)
      if (state.recentOutput !== undefined && SECRET_PROMPT.test(state.recentOutput)) {
        return `<redacted, ${text.length} chars>`
      }
    } catch {
      return text
    }
    return text
  }

  private clip(reason: string): string {
    const max = this.service.settings.approval.reasonMaxChars
    if (reason.length <= max) return reason
    return `${reason.slice(0, Math.max(0, max - 1))}…`
  }
}

export function isShortAnswer(text: string): boolean {
  const trimmed = text.trim().toLowerCase()
  if (SHORT_ANSWERS.has(trimmed)) return true
  return trimmed.length === 1
}

/** User-facing session label. Never dump a command-profile launch line as the host. */
export function sessionLabel(target: SessionTarget | undefined): string {
  if (target === undefined) return 'this session'
  const name = target.profileName.trim()
  if (name.length > 0) return `profile "${name}"`
  const dest = target.target.trim()
  if (dest.length > 0 && !dest.includes(' ')) return dest
  return 'this session'
}

function connectLabel(
  profile: { name: string; host: string; connectType: string } | undefined,
  profileId: string,
): string {
  if (profile === undefined) return `profile ${profileId}`
  if (profile.connectType === 'command' || profile.host.length === 0) {
    return `profile "${profile.name}"`
  }
  return `profile "${profile.name}" (${profile.host})`
}

function formatFlagged(rules: string[]): string {
  if (rules.length === 0) return ''
  const details = rules.filter(rule => rule !== 'production-not-readonly').map(shortRule)
  if (rules.includes('production-not-readonly')) {
    const extra = details.length > 0 ? ` Also: ${details.join(', ')}.` : ''
    return `\nNot a read-only command on a production host.${extra}`
  }
  return `\nFlagged: ${details.join(', ')}.`
}

function shortRule(rule: string): string {
  const match = /^[\w-]+ \((.+)\)$/.exec(rule)
  return match?.[1] ?? rule
}

function capitalize(value: string): string {
  if (value.length === 0) return value
  return value[0]!.toUpperCase() + value.slice(1)
}
