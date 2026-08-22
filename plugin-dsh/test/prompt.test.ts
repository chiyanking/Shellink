import { describe, expect, it } from 'vitest'
import { SHELLINK_PROMPT } from '../src/prompt.js'

describe('SHELLINK_PROMPT', () => {
  it('teaches the tool workflow and does not tell the model to use the CLI', () => {
    expect(SHELLINK_PROMPT).toContain('shellink_session_open')
    expect(SHELLINK_PROMPT).toContain('WAITING_INPUT')
    expect(SHELLINK_PROMPT).toContain('MANUAL')
    expect(SHELLINK_PROMPT).toContain('password_ref')
    expect(SHELLINK_PROMPT).not.toContain('Do not implement the Unix socket')
    expect(SHELLINK_PROMPT).not.toMatch(/shellink session exec/)
    expect(SHELLINK_PROMPT).toContain('require user approval')
    expect(SHELLINK_PROMPT).toContain('do not rewrite')
  })
})
