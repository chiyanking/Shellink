import { describe, expect, it } from 'vitest'
import {
  MissingCredentialError,
  SessionNotReadyError,
  ShellinkBinaryMissingError,
  ShellinkHttpError,
  formatRpcError,
  notReadyMessage,
} from '../src/errors.js'

describe('notReadyMessage', () => {
  it('explains MANUAL takeover', () => {
    expect(notReadyMessage('s1', 'WAITING_INPUT', 'MANUAL')).toContain('MANUAL')
    expect(notReadyMessage('s1', 'WAITING_INPUT', 'MANUAL')).toContain('s1')
  })

  it('explains each blocking state', () => {
    expect(notReadyMessage('s1', 'DISCONNECTED', 'AUTO')).toContain('DISCONNECTED')
    expect(notReadyMessage('s1', 'CONNECTING', 'AUTO')).toContain('CONNECTING')
    expect(notReadyMessage('s1', 'OUTPUTTING', 'AUTO')).toContain('OUTPUTTING')
    expect(notReadyMessage('s1', 'IDLE', 'AUTO')).toContain('IDLE')
    expect(notReadyMessage('s1', 'WAITING_INPUT', 'AUTO')).toContain('WAITING_INPUT')
  })
})

describe('formatRpcError', () => {
  it('keeps typed errors and HTTP status codes', () => {
    expect(formatRpcError(new MissingCredentialError('PROD_PW'))).toContain('PROD_PW')
    expect(formatRpcError(new ShellinkBinaryMissingError())).toContain('PATH')
    expect(formatRpcError(new SessionNotReadyError('s1', 'IDLE', 'AUTO', 'idle now'))).toBe('idle now')
    expect(formatRpcError(new ShellinkHttpError(404, 'missing'))).toBe('HTTP_404: missing')
    expect(formatRpcError(new Error('plain'))).toBe('plain')
    expect(formatRpcError(42)).toBe('42')
  })
})
