import { spawn } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { ShellinkBinaryMissingError } from './errors.js'
import { ShellinkHttpClient } from './client.js'

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** Walk PATH for an executable `shellink` binary. */
export function findShellinkBinary(env: NodeJS.ProcessEnv = process.env): string | undefined {
  const pathValue = env.PATH ?? ''
  const name = process.platform === 'win32' ? 'shellink.exe' : 'shellink'
  for (const dir of pathValue.split(path.delimiter)) {
    if (!dir) continue
    const candidate = path.join(dir, name)
    try {
      fs.accessSync(candidate, fs.constants.X_OK)
      return candidate
    } catch {
      // keep looking
    }
  }
  return undefined
}

export function resolveBaseUrl(baseUrl?: string, env: NodeJS.ProcessEnv = process.env): string {
  const configured = baseUrl ?? env.SHELLINK_HTTP_URL
  const resolved = configured ?? `http://127.0.0.1:${env.SHELLINK_PORT || '7070'}`
  return new URL(resolved).toString().replace(/\/$/, '')
}

/** Return an HTTP client when the Go daemon health endpoint is available. */
export async function connectExisting(
  baseUrl: string,
  probeTimeoutMs: number,
  requestTimeoutMs = probeTimeoutMs,
  fetchImpl: typeof fetch = fetch,
): Promise<ShellinkHttpClient | null> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), probeTimeoutMs)
  try {
    const response = await fetchImpl(`${baseUrl}/shellink/healthz`, { signal: controller.signal })
    if (!response.ok) return null
    return new ShellinkHttpClient(baseUrl, requestTimeoutMs, fetchImpl)
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

/** Connect to the Go daemon, optionally spawning `shellink server start`. */
export async function ensureDaemon(options: {
  baseUrl: string
  autoStart: boolean
  timeoutMs: number
  requestTimeoutMs?: number
  binary?: string
  fetchImpl?: typeof fetch
}): Promise<ShellinkHttpClient> {
  const requestTimeoutMs = options.requestTimeoutMs ?? 30_000
  const fetchImpl = options.fetchImpl ?? fetch
  const existing = await connectExisting(options.baseUrl, 1000, requestTimeoutMs, fetchImpl)
  if (existing) return existing
  if (!options.autoStart) {
    throw new Error(`Shellink daemon is not running at ${options.baseUrl}. Start it with \`shellink server start\` or set autoStartDaemon: true.`)
  }
  const binary = options.binary ?? findShellinkBinary()
  if (!binary) throw new ShellinkBinaryMissingError()
  const child = spawn(binary, ['server', 'start'], { detached: true, stdio: 'ignore', env: process.env })
  child.unref()
  const deadline = Date.now() + options.timeoutMs
  while (Date.now() < deadline) {
    await sleep(100)
    const client = await connectExisting(options.baseUrl, 1000, requestTimeoutMs, fetchImpl)
    if (client) return client
  }
  throw new Error(`Timed out starting the shellink daemon via \`${binary} server start\`.`)
}
