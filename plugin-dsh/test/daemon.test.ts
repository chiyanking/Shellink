import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ShellinkHttpClient } from '../src/client.js'
import { ShellinkBinaryMissingError } from '../src/errors.js'
import { connectExisting, ensureDaemon, findShellinkBinary, resolveBaseUrl } from '../src/daemon.js'

describe('resolveBaseUrl', () => {
  it('prefers an explicit URL', () => {
    expect(resolveBaseUrl('http://127.0.0.1:8080/')).toBe('http://127.0.0.1:8080')
  })

  it('uses the HTTP URL or port from the environment', () => {
    expect(resolveBaseUrl(undefined, { SHELLINK_HTTP_URL: 'http://localhost:9000/' }))
      .toBe('http://localhost:9000')
    expect(resolveBaseUrl(undefined, { SHELLINK_PORT: '8088' }))
      .toBe('http://127.0.0.1:8088')
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
})

describe('HTTP daemon connection', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('returns null when the health endpoint is unavailable', async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => new Response('', { status: 503 }))
    expect(await connectExisting('http://127.0.0.1:1', 50, 50, fetchImpl)).toBeNull()
  })

  it('returns a client when the Go health endpoint responds', async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => Response.json({ ok: true }))
    const client = await connectExisting('http://127.0.0.1:7070', 50, 500, fetchImpl)
    expect(client).toBeInstanceOf(ShellinkHttpClient)
    expect(fetchImpl).toHaveBeenCalledWith(
      'http://127.0.0.1:7070/shellink/healthz',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  it('throws when autoStart is false and no daemon is listening', async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => new Response('', { status: 503 }))
    await expect(ensureDaemon({
      baseUrl: 'http://127.0.0.1:1',
      autoStart: false,
      timeoutMs: 50,
      fetchImpl,
    })).rejects.toThrow(/not running/)
  })

  it('times out when the spawned CLI never starts its HTTP server', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-spawn-'))
    const binary = path.join(dir, 'shellink')
    fs.writeFileSync(binary, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    const fetchImpl = vi.fn<typeof fetch>(async () => new Response('', { status: 503 }))
    try {
      await expect(ensureDaemon({
        baseUrl: 'http://127.0.0.1:1',
        autoStart: true,
        timeoutMs: 250,
        binary,
        fetchImpl,
      })).rejects.toThrow(/Timed out starting/)
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })

  it('connects after starting the CLI', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-spawn-ok-'))
    const binary = path.join(dir, 'shellink')
    fs.writeFileSync(binary, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    let attempts = 0
    const fetchImpl = vi.fn<typeof fetch>(async () => {
      attempts += 1
      return attempts < 2 ? new Response('', { status: 503 }) : Response.json({ ok: true })
    })
    try {
      const client = await ensureDaemon({
        baseUrl: 'http://127.0.0.1:1',
        autoStart: true,
        timeoutMs: 500,
        binary,
        fetchImpl,
      })
      expect(client).toBeInstanceOf(ShellinkHttpClient)
      expect(attempts).toBeGreaterThan(1)
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })

  it('throws when autoStart cannot find a binary', async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => new Response('', { status: 503 }))
    await expect(ensureDaemon({
      baseUrl: 'http://127.0.0.1:1',
      autoStart: true,
      timeoutMs: 50,
      binary: '',
      fetchImpl,
    })).rejects.toBeInstanceOf(ShellinkBinaryMissingError)
  })
})
