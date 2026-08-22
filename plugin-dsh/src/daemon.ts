import { spawn } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { SocketClient, shellinkPaths } from '@shellink/client'
import { ShellinkBinaryMissingError } from './errors.js'

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

/** Try to connect to an already-running daemon. */
export async function connectExisting(
  socketPath: string,
  probeTimeoutMs: number,
  requestTimeoutMs = probeTimeoutMs,
): Promise<SocketClient | null> {
  const client = new SocketClient(socketPath, requestTimeoutMs)
  try {
    await client.connect(probeTimeoutMs)
    return client
  } catch {
    client.close()
    return null
  }
}

/** Resolve the socket path from config or the process environment. */
export function resolveSocketPath(socketPath?: string, env: NodeJS.ProcessEnv = process.env): string {
  if (socketPath) return path.resolve(socketPath)
  return shellinkPaths(env).socketPath
}

/** Connect to the daemon, optionally spawning `shellink server start`. */
export async function ensureDaemon(options: {
  socketPath: string
  autoStart: boolean
  timeoutMs: number
  requestTimeoutMs?: number
  binary?: string
}): Promise<SocketClient> {
  const requestTimeoutMs = options.requestTimeoutMs ?? 30_000
  const existing = await connectExisting(options.socketPath, 1000, requestTimeoutMs)
  if (existing) return existing
  if (!options.autoStart) {
    throw new Error(`Shellink daemon is not running at ${options.socketPath}. Start it with \`shellink server start\` or set autoStartDaemon: true.`)
  }
  const binary = options.binary ?? findShellinkBinary()
  if (!binary) throw new ShellinkBinaryMissingError()
  const child = spawn(binary, ['server', 'start'], { detached: true, stdio: 'ignore', env: process.env })
  child.unref()
  const deadline = Date.now() + options.timeoutMs
  while (Date.now() < deadline) {
    await sleep(100)
    const client = await connectExisting(options.socketPath, 1000, requestTimeoutMs)
    if (client) return client
  }
  throw new Error(`Timed out starting the shellink daemon via \`${binary} server start\`.`)
}
