/** Injectable copy for SocketClient RPC errors. */
export interface SocketClientMessages {
  daemonClosed: string
  daemonNotConnected: string
  requestTimedOut: (method: string) => string
}

/** English defaults used when a caller does not inject locale-specific copy. */
export const defaultSocketClientMessages: SocketClientMessages = {
  daemonClosed: 'Daemon connection closed',
  daemonNotConnected: 'Not connected to the daemon',
  requestTimedOut: (method) => `${method} request timed out`,
}
