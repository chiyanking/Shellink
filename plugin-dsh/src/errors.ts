import type { InteractionMode, SessionState } from './types.js'

/** Session is not in AUTO + WAITING_INPUT, so exec/transfer/edit cannot start. */
export class SessionNotReadyError extends Error {
  constructor(
    public readonly sessionId: string,
    public readonly state: SessionState,
    public readonly mode: InteractionMode,
    message: string,
  ) {
    super(message)
    this.name = 'SessionNotReadyError'
  }
}

/** A credential reference has no configured value. */
export class MissingCredentialError extends Error {
  constructor(public readonly ref: string) {
    super(
      `credential ${ref} is not configured. Store it in $DSH_HOME/.credentials.yaml or the process environment before creating or updating a profile.`,
    )
    this.name = 'MissingCredentialError'
  }
}

/** The local shellink binary could not be found. */
export class ShellinkBinaryMissingError extends Error {
  constructor() {
    super(
      'shellink is not on PATH. Install the CLI from https://github.com/jie123108/Shellink (install.sh or npm run build) and retry.',
    )
    this.name = 'ShellinkBinaryMissingError'
  }
}

export class ShellinkHttpError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message)
    this.name = 'ShellinkHttpError'
  }
}

/** Format a daemon or client failure for the model. */
export function formatRpcError(error: unknown): string {
  if (error instanceof SessionNotReadyError || error instanceof MissingCredentialError || error instanceof ShellinkBinaryMissingError) {
    return error.message
  }
  if (error instanceof ShellinkHttpError) return `HTTP_${error.status}: ${error.message}`
  if (error instanceof Error) return error.message
  return String(error)
}

/** Actionable text when a session cannot accept AI commands. */
export function notReadyMessage(sessionId: string, state: SessionState, mode: InteractionMode): string {
  if (mode === 'MANUAL') {
    return `Session ${sessionId} is in MANUAL mode; a human currently controls the terminal. Wait until they switch it back to AUTO (shellink_session_mode with mode AUTO, or the Shellink UI) before sending commands.`
  }
  if (state === 'DISCONNECTED') {
    return `Session ${sessionId} is DISCONNECTED. Open a new session with shellink_session_open.`
  }
  if (state === 'CONNECTING') {
    return `Session ${sessionId} is still CONNECTING (login or jump-host prompts may be pending). Wait for WAITING_INPUT, or ask a human to finish login in MANUAL mode.`
  }
  if (state === 'OUTPUTTING') {
    return `Session ${sessionId} is OUTPUTTING. Wait until it reaches WAITING_INPUT before exec, upload, download, or edit.`
  }
  if (state === 'IDLE') {
    return `Session ${sessionId} is IDLE (no prompt detected). Inspect shellink_session_history / shellink_session_state and send shellink_session_input only if a program is waiting for an answer.`
  }
  return `Session ${sessionId} is ${state} in ${mode} mode; commands can run only while AUTO and WAITING_INPUT.`
}
