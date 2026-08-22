import Schema from '@deepseek-ai/schemastery'

export type ApprovalMode = 'enforce' | 'off'
export type ApprovalTier = 'standard' | 'production'
export type ConnectGate = 'always' | 'production-only' | 'off'
export type RememberMode = 'connect+command' | 'connect' | 'off'

/** User-facing approval-gate settings. */
export interface ApprovalConfig {
  mode: ApprovalMode
  defaultTier: ApprovalTier
  productionPatterns: string[]
  productionProfileIds: string[]
  connect: ConnectGate
  remember: RememberMode
  memoMaxEntries: number
  extraDangerPatterns: string[]
  allowPatterns: string[]
  readOnlyExtra: string[]
  gateProfileDelete: boolean
  gateSessionClose: boolean
  reasonMaxChars: number
}

/** Regexes compiled once at plugin load. Invalid patterns throw. */
export interface CompiledApproval {
  extraDanger: RegExp[]
  allow: RegExp[]
  production: RegExp[]
}

/** Plugin configuration for the Shellink daemon client. */
export interface Config {
  socketPath?: string
  autoStartDaemon: boolean
  daemonReadyTimeoutMs: number
  requestTimeoutMs: number
  waitReadyTimeoutMs: number
  execDefaultTimeoutMs: number
  uploadMaxBytes: number
  profileWrites: boolean
  approval: ApprovalConfig
}

const ApprovalConfigSchema: Schema<ApprovalConfig> = Schema.object({
  mode: Schema.union(['enforce', 'off']).default('enforce'),
  defaultTier: Schema.union(['standard', 'production']).default('standard'),
  productionPatterns: Schema.array(Schema.string()).default(['(?i)prod', '(?i)prd']),
  productionProfileIds: Schema.array(Schema.string()).default([]),
  connect: Schema.union(['always', 'production-only', 'off']).default('always'),
  remember: Schema.union(['connect+command', 'connect', 'off']).default('connect+command'),
  memoMaxEntries: Schema.number().default(200),
  extraDangerPatterns: Schema.array(Schema.string()).default([]),
  allowPatterns: Schema.array(Schema.string()).default([]),
  readOnlyExtra: Schema.array(Schema.string()).default([]),
  gateProfileDelete: Schema.boolean().default(false),
  gateSessionClose: Schema.boolean().default(false),
  reasonMaxChars: Schema.number().default(400),
})

/** Schemastery schema with defaults. */
export const Config: Schema<Config> = Schema.object({
  socketPath: Schema.string(),
  autoStartDaemon: Schema.boolean().default(true),
  daemonReadyTimeoutMs: Schema.number().default(10_000),
  requestTimeoutMs: Schema.number().default(30_000),
  waitReadyTimeoutMs: Schema.number().default(30_000),
  execDefaultTimeoutMs: Schema.number().default(20_000),
  uploadMaxBytes: Schema.number().default(6_291_456),
  profileWrites: Schema.boolean().default(true),
  approval: ApprovalConfigSchema,
})

/**
 * Compile a user-supplied pattern. A leading `(?imsu)` prefix is translated
 * into JS flags because JavaScript does not accept Perl-style inline flags.
 */
export function compileUserRegex(source: string, label: string): RegExp {
  try {
    const match = /^\(\?([imsu]+)\)/.exec(source)
    if (match) {
      return new RegExp(source.slice(match[0].length), match[1])
    }
    return new RegExp(source)
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    throw new Error(`invalid approval regex ${label}: ${source}: ${detail}`)
  }
}

/** Compile every user regex in the approval config. Throws on the first invalid pattern. */
export function compileApproval(approval: ApprovalConfig): CompiledApproval {
  return {
    extraDanger: approval.extraDangerPatterns.map((source, index) => (
      compileUserRegex(source, `extraDangerPatterns[${index}]`)
    )),
    allow: approval.allowPatterns.map((source, index) => (
      compileUserRegex(source, `allowPatterns[${index}]`)
    )),
    production: approval.productionPatterns.map((source, index) => (
      compileUserRegex(source, `productionPatterns[${index}]`)
    )),
  }
}
