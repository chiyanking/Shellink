import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { shellinkPaths } from '../src/paths.js'

const original = {
  SHELLINK_HOME: process.env.SHELLINK_HOME,
  SHELLINK_SOCKET: process.env.SHELLINK_SOCKET,
  SHELLINK_LOG: process.env.SHELLINK_LOG,
  XDG_RUNTIME_DIR: process.env.XDG_RUNTIME_DIR,
  TMPDIR: process.env.TMPDIR,
}

afterEach(() => {
  for (const [key, value] of Object.entries(original)) {
    if (value === undefined) delete process.env[key]
    else process.env[key] = value
  }
})

describe('shellinkPaths', () => {
  it('resolves explicit SHELLINK_HOME, SHELLINK_SOCKET, and SHELLINK_LOG', () => {
    const paths = shellinkPaths({
      SHELLINK_HOME: '/tmp/shellink-home',
      SHELLINK_SOCKET: '/tmp/custom.sock',
      SHELLINK_LOG: '/tmp/custom.log',
    })
    expect(paths.home).toBe(path.resolve('/tmp/shellink-home'))
    expect(paths.socketPath).toBe(path.resolve('/tmp/custom.sock'))
    expect(paths.logPath).toBe(path.resolve('/tmp/custom.log'))
    expect(paths.pidPath).toBe(path.join(path.resolve('/tmp/shellink-home'), 'shellink.pid'))
  })

  it('defaults home to ~/.Shellink when SHELLINK_HOME is absent', () => {
    const paths = shellinkPaths({})
    expect(paths.home).toBe(path.join(os.homedir(), '.Shellink'))
    expect(paths.logPath).toBe(path.join(os.homedir(), '.Shellink', 'shellink.log'))
  })

  it('uses XDG_RUNTIME_DIR on linux when SHELLINK_SOCKET is absent', () => {
    const paths = shellinkPaths({ XDG_RUNTIME_DIR: '/run/user/1000' }, 'linux')
    expect(paths.socketPath).toBe(path.join('/run/user/1000', 'shellink', 'shellink.sock'))
  })

  it('uses /tmp on linux when XDG_RUNTIME_DIR is absent', () => {
    const paths = shellinkPaths({}, 'linux')
    expect(paths.socketPath).toMatch(/\/tmp\/shellink-\d+\/shellink\.sock$/)
  })

  it('uses TMPDIR on darwin when SHELLINK_SOCKET is absent', () => {
    const paths = shellinkPaths({ TMPDIR: '/tmp' }, 'darwin')
    expect(paths.socketPath.startsWith(path.join('/tmp', 'shellink-'))).toBe(true)
    expect(paths.socketPath.endsWith('shellink.sock')).toBe(true)
  })

  it('falls back to os.tmpdir on darwin when TMPDIR is absent', () => {
    const paths = shellinkPaths({}, 'darwin')
    expect(paths.socketPath.startsWith(path.join(os.tmpdir(), 'shellink-'))).toBe(true)
    expect(paths.socketPath.endsWith('shellink.sock')).toBe(true)
  })

  it('falls back to uid 0 when process.getuid is missing', () => {
    const getuid = process.getuid
    Object.defineProperty(process, 'getuid', { configurable: true, value: undefined })
    try {
      const paths = shellinkPaths({ TMPDIR: '/tmp' }, 'darwin')
      expect(paths.socketPath).toBe(path.join('/tmp', 'shellink-0', 'shellink.sock'))
    } finally {
      Object.defineProperty(process, 'getuid', { configurable: true, value: getuid })
    }
  })
})
