import os from 'node:os'
import path from 'node:path'

/** Resolved on-disk locations for one Shellink daemon. */
export interface ShellinkPaths {
  home: string
  socketPath: string
  logPath: string
  pidPath: string
}

/** Resolve home, socket, log, and pid paths from the process environment. */
export function shellinkPaths(env: NodeJS.ProcessEnv = process.env, platform: NodeJS.Platform = process.platform): ShellinkPaths {
  const uid = typeof process.getuid === 'function' ? process.getuid() : 0
  const home = env.SHELLINK_HOME
    ? path.resolve(env.SHELLINK_HOME)
    : path.join(os.homedir(), '.Shellink')
  const socketPath = env.SHELLINK_SOCKET
    ? path.resolve(env.SHELLINK_SOCKET)
    : platform === 'darwin'
      ? path.join(env.TMPDIR ?? os.tmpdir(), `shellink-${uid}`, 'shellink.sock')
      : env.XDG_RUNTIME_DIR
        ? path.join(env.XDG_RUNTIME_DIR, 'shellink', 'shellink.sock')
        : path.join('/tmp', `shellink-${uid}`, 'shellink.sock')
  return {
    home,
    socketPath,
    logPath: path.resolve(env.SHELLINK_LOG ?? path.join(home, 'shellink.log')),
    pidPath: path.join(home, 'shellink.pid'),
  }
}
