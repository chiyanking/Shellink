/**
 * Production-tier matching and session→profile cache for the approval gate.
 * @module shellink-dsh/tier
 */

import type { ApprovalConfig, CompiledApproval } from './config.js'

export interface SessionTarget {
  profileId: string
  profileName: string
  target: string
}

export interface ProductionFields {
  id?: string
  name?: string
  host?: string
  target?: string
  uniqueId?: string | null
}

/** In-process map from Shellink session id to the profile used to open it. */
export class SessionTargetCache {
  private readonly map = new Map<string, SessionTarget>()

  get(id: string): SessionTarget | undefined {
    return this.map.get(id)
  }

  set(id: string, target: SessionTarget): void {
    this.map.set(id, target)
  }

  delete(id: string): void {
    this.map.delete(id)
  }

  clear(): void {
    this.map.clear()
  }

  ingest(rows: Array<SessionTarget & { id: string }>): void {
    for (const row of rows) {
      this.map.set(row.id, {
        profileId: row.profileId,
        profileName: row.profileName,
        target: row.target,
      })
    }
  }
}

/** True when the profile/session fields match the configured production rules. */
export function isProductionTarget(
  fields: ProductionFields,
  approval: ApprovalConfig,
  compiled: CompiledApproval,
): boolean {
  if (approval.defaultTier === 'production') return true
  if (fields.id !== undefined && approval.productionProfileIds.includes(fields.id)) return true
  const haystacks = [fields.name, fields.host, fields.target, fields.uniqueId]
    .filter((value): value is string => value !== undefined && value !== null && value.length > 0)
  return compiled.production.some(pattern => haystacks.some(value => pattern.test(value)))
}
