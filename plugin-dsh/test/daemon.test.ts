import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { SocketClient } from '@shellink/client'
import { ShellinkBinaryMissingError } from '../src/errors.js'
import { connectExisting, ensureDaemon, findShellinkBinary, resolveSocketPath } from '../src/daemon.js'

describe('resolveSocketPath', () => {
  it('prefers an explicit path', () => {
    expect(resolveSocketPath('/tmp/custom.sock')).toBe(path.resolve('/tmp/custom.sock'))
  })

  it('falls back to SHELLINK_SOCKET', () => {
    expect(resolveSocketPath(undefined, { SHELLINK_SOCKET: '/tmp/from-env.sock' }))
      .toBe(path.resolve('/tmp/from-env.sock'))
  })
})

describe('findShellinkBinary', () => {
  it('finds an executable on PATH', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-bin-'))
    const binary = path.join(dir, 'shellink')
    fs.writeFileSync(binary, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    expect(findShellinkBinary({ PATH: dir })).toBe(binary)
    fs.rmSync(dir, { recursive: true, force: true })
  })

  it('returns undefined when PATH is empty', () => {
    expect(findShellinkBinary({ PATH: '' })).toBeUndefined()
  })

  it('skips a non-executable candidate and keeps looking', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-bin-'))
    fs.writeFileSync(path.join(dir, 'shellink'), 'not executable\n', { mode: 0o644 })
    expect(findShellinkBinary({ PATH: dir })).toBeUndefined()
    fs.rmSync(dir, { recursive: true, force: true })
  })

  it('skips empty PATH segments', () => {
    expect(findShellinkBinary({ PATH: `${path.delimiter}/no-such-shellink-dir` })).toBeUndefined()
  })
})

describe('ensureDaemon', () => {
  it('returns null from connectExisting when the socket is missing', async () => {
    expect(await connectExisting(path.join(os.tmpdir(), 'no-such-shellink.sock'), 50)).toBeNull()
  })

  it('throws when autoStart is false and nothing is listening', async () => {
    await expect(ensureDaemon({
      socketPath: path.join(os.tmpdir(), 'no-such-shellink.sock'),
      autoStart: false,
      timeoutMs: 50,
    })).rejects.toThrow(/not running/)
  })

  it('times out when the spawned CLI never opens the socket', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-spawn-'))
    const binary = path.join(dir, 'shellink')
    fs.writeFileSync(binary, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    try {
      await expect(ensureDaemon({
        socketPath: path.join(dir, 'missing.sock'),
        autoStart: true,
        timeoutMs: 250,
        binary,
      })).rejects.toThrow(/Timed out starting/)
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })

  it('returns an already-running daemon', async () => {
    const connect = vi.spyOn(SocketClient.prototype, 'connect').mockResolvedValue({
      protocolVersion: 1,
      serverVersion: 'test',
    } as never)
    try {
      const existing = await connectExisting('/tmp/already.sock', 50)
      expect(existing).toBeInstanceOf(SocketClient)
      expect(await ensureDaemon({
        socketPath: '/tmp/already.sock',
        autoStart: false,
        timeoutMs: 50,
      })).toBeInstanceOf(SocketClient)
    } finally {
      connect.mockRestore()
    }
  })

  it('connects after spawning the CLI', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-spawn-ok-'))
    const binary = path.join(dir, 'shellink')
    fs.writeFileSync(binary, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    let attempts = 0
    const connect = vi.spyOn(SocketClient.prototype, 'connect').mockImplementation(async () => {
      attempts += 1
      if (attempts === 1) throw new Error('not yet')
      return { protocolVersion: 1, serverVersion: 'test' } as never
    })
    try {
      const client = await ensureDaemon({
        socketPath: path.join(dir, 'later.sock'),
        autoStart: true,
        timeoutMs: 500,
        binary,
      })
      expect(client).toBeInstanceOf(SocketClient)
      expect(attempts).toBeGreaterThan(1)
    } finally {
      connect.mockRestore()
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })

  it('throws ShellinkBinaryMissingError when autoStart cannot find the CLI', async () => {
    const previous = process.env.PATH
    process.env.PATH = ''
    try {
      await expect(ensureDaemon({
        socketPath: path.join(os.tmpdir(), 'no-such-shellink.sock'),
        autoStart: true,
        timeoutMs: 50,
      })).rejects.toBeInstanceOf(ShellinkBinaryMissingError)
    } finally {
      process.env.PATH = previous
    }
  })
})
