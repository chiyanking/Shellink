/**
 * Heuristic remote-command classifier used by the approval gate.
 * Quote-aware split, wrapper peeling, then layered danger / read-only rules.
 * @module shellink-dsh/danger
 */

export type CommandRisk = 'safe' | 'danger'
export type PathKind = 'ok' | 'sensitive' | 'secret'

export interface CommandSegment {
  text: string
  redirects: string[]
  piped: boolean
}

export interface ClassifyOptions {
  extraDanger?: readonly RegExp[]
  allow?: readonly RegExp[]
  readOnlyExtra?: readonly string[]
  production?: boolean
}

export interface ClassifyResult {
  risk: CommandRisk
  rules: string[]
  programs: string[]
  readonly: boolean
}

const WRAPPERS = new Set([
  'sudo', 'doas', 'env', 'nohup', 'time', 'nice', 'ionice', 'timeout',
  'stdbuf', 'xargs', 'command', 'builtin', 'exec', 'setsid', 'su',
])

const WRAPPER_VALUE_FLAGS = new Set([
  '-u', '--user', '-g', '--group', '-h', '--host', '-p', '--prompt',
  '-r', '--role', '-t', '--type', '-C', '--close-from', '-D', '--chdir',
  '--unset', '--chdir', '-k', '--kill-after', '-s', '--signal',
  '-n', '--adjustment', '-c', '-I', '-i', '-L', '-l', '-P', '-E', '-e',
  '--command', '--shell', '--whitelist-environment', '-o', '--output',
  '-f', '--format', '-S', '--split-string',
])

const IDENTITY_WRAPPERS = new Set(['sudo', 'su', 'doas'])

const DESTRUCTIVE_FS = new Set([
  'rm', 'rmdir', 'shred', 'unlink', 'wipefs', 'dd', 'fdisk', 'parted', 'truncate', 'mv', 'tee',
])

const PROCESS_CONTROL = new Set(['kill', 'pkill', 'killall', 'skill'])

const POWER = new Set(['reboot', 'shutdown', 'halt', 'poweroff'])

const PERMISSION = new Set([
  'chmod', 'chown', 'chgrp', 'chattr', 'setfacl',
  'useradd', 'userdel', 'usermod', 'passwd', 'chpasswd', 'visudo',
  'sudo', 'su', 'doas',
])

const PACKAGE_ALWAYS = new Set([
  'apt', 'apt-get', 'aptitude', 'apt-add-repository',
  'yum', 'dnf', 'apk', 'pacman', 'zypper', 'brew', 'snap',
])

const PACKAGE_SUB = new Set(['pip', 'pip3', 'pipx', 'npm', 'pnpm', 'yarn'])
const PACKAGE_SUB_ACTIONS = new Set(['install', 'uninstall', 'remove', 'publish', 'add'])

const INTERPRETERS = new Set(['sh', 'bash', 'zsh', 'ksh', 'dash', 'fish'])
const SCRIPT_LANGS = new Set(['python', 'python2', 'python3', 'perl', 'node', 'nodejs', 'ruby'])
const FETCHERS = new Set(['curl', 'wget'])

const FIREWALL = new Set(['iptables', 'ip6tables', 'nft', 'ufw', 'firewall-cmd', 'ipset'])
const IP_OBJECTS = new Set(['route', 'addr', 'link', 'rule'])
const IP_WRITES = new Set(['add', 'del', 'delete', 'change', 'replace', 'flush', 'set'])

const DB_CLIENTS = new Set(['mysql', 'psql', 'mongosh', 'mongo', 'redis-cli', 'sqlite3'])
const SQL_WRITE = /\b(DROP|DELETE|TRUNCATE|ALTER|UPDATE|GRANT|FLUSHALL|FLUSHDB|CONFIG\s+SET)\b/i

const SAFE_REDIRECTS = new Set(['/dev/null', '&1', '&2', '1', '2'])

const READ_ONLY_PROGRAMS = new Set([
  'ls', 'cat', 'head', 'tail', 'wc', 'grep', 'egrep', 'fgrep', 'rg', 'find',
  'stat', 'file', 'du', 'df', 'free', 'uname', 'uptime', 'hostname', 'whoami',
  'id', 'groups', 'date', 'printenv', 'env', 'ps', 'pgrep', 'pidof', 'lsof',
  'netstat', 'ss', 'ifconfig', 'ping', 'dig', 'nslookup', 'echo', 'printf',
  'pwd', 'which', 'whereis', 'type', 'md5sum', 'sha256sum', 'sha1sum',
  'diff', 'cmp', 'sort', 'uniq', 'cut', 'awk', 'sed', 'tr', 'jq', 'xxd', 'od',
  'strings', 'readlink', 'realpath', 'basename', 'dirname', 'tree',
  'journalctl', 'dmesg', 'last', 'w', 'who', 'vmstat', 'iostat', 'nproc',
  'lscpu', 'lsblk', 'lsmod', 'rpm', 'dpkg', 'true', 'false',
])

const DOCKER_READ = new Set(['ps', 'images', 'logs', 'inspect', 'stats'])
const DOCKER_WRITE = new Set(['stop', 'kill', 'rm', 'rmi', 'prune', 'restart', 'down'])
const KUBECTL_READ = new Set(['get', 'describe', 'logs', 'top', 'explain'])
const SYSTEMCTL_READ = new Set(['status', 'is-active', 'is-enabled', 'list-units', 'list-unit-files', 'show', 'cat'])
const SYSTEMCTL_WRITE = new Set(['stop', 'restart', 'kill', 'disable', 'mask'])
const GIT_READ = new Set(['status', 'log', 'diff', 'show', 'remote', 'rev-parse'])
const SERVICE_WRITE = new Set(['stop', 'restart'])
const PM2_WRITE = new Set(['stop', 'delete', 'restart', 'kill'])

const SENSITIVE_PATH = /(?:^|[\s"'=:])(\/etc(?:\/|\s|$)|\/boot(?:\/|\s|$)|\/sys(?:\/|\s|$)|\/proc\/sys(?:\/|\s|$)|\/dev\/sd[a-z]|\/dev\/nvme|\/root(?:\/|\s|$)|~\/\.ssh|\/\.ssh(?:\/|\s|$)|authorized_keys|\/etc\/passwd|\/etc\/shadow|\/etc\/sudoers|\/etc\/systemd|\/usr\/lib\/systemd|\/lib\/systemd|\/usr\/bin(?:\/|\s|$)|\/usr\/local\/bin(?:\/|\s|$)|\/var\/lib(?:\/|\s|$)|\/etc\/cron|\/var\/spool\/cron)/i

const SENSITIVE_WRITE_PREFIXES = [
  '/etc',
  '/usr',
  '/boot',
  '/lib/systemd',
  '/usr/lib/systemd',
  '/var/spool/cron',
]

const SECRET_NAME = /(?:^|\/)(?:id_rsa|id_ed25519|id_ecdsa|id_dsa|credentials)(?:$|\.)/i
const SECRET_EXT = /\.(?:pem|key)$/i
const SECRET_ENV = /(?:^|\/)\.env(?:$|\.|[-_])/i

/** Split on unquoted `;` `&&` `||` `|` `&` / newlines; expand `$( )` and backticks. */
export function splitSegments(command: string): CommandSegment[] {
  const segments: CommandSegment[] = []
  const extras: CommandSegment[] = []
  let current = ''
  let redirects: string[] = []
  let quote: "'" | '"' | null = null
  let i = 0

  const flush = (piped: boolean): void => {
    const text = current.trim()
    if (text.length > 0 || redirects.length > 0) {
      segments.push({ text, redirects, piped })
    }
    current = ''
    redirects = []
  }

  while (i < command.length) {
    const c = command[i] ?? ''
    if (quote !== null) {
      if (c === quote && (quote === "'" || command[i - 1] !== '\\')) quote = null
      current += c
      i += 1
      continue
    }
    if (c === "'" || c === '"') {
      quote = c
      current += c
      i += 1
      continue
    }
    if (c === '\n' || c === ';') {
      flush(false)
      i += 1
      continue
    }
    if (c === '&' && command[i + 1] === '&') {
      flush(false)
      i += 2
      continue
    }
    if (c === '|' && command[i + 1] === '|') {
      flush(false)
      i += 2
      continue
    }
    if (c === '|') {
      flush(true)
      i += 1
      continue
    }
    if (c === '&') {
      flush(false)
      i += 1
      continue
    }
    if (c === '>') {
      const append = command[i + 1] === '>'
      i += append ? 2 : 1
      while (command[i] === ' ' || command[i] === '\t') i += 1
      if (command[i] === '&') {
        let dest = '&'
        i += 1
        while (i < command.length && /[0-9]/.test(command[i] ?? '')) {
          dest += command[i]
          i += 1
        }
        redirects.push(dest)
        continue
      }
      const word = readWord(command, i)
      redirects.push(word.value)
      i = word.end
      continue
    }
    if (c === '$' && command[i + 1] === '(') {
      const inner = readBalanced(command, i + 2)
      current += command.slice(i, inner.end)
      extras.push(...splitSegments(inner.inner))
      i = inner.end
      continue
    }
    if (c === '`') {
      const inner = readUntil(command, i + 1, '`')
      current += command.slice(i, inner.end)
      extras.push(...splitSegments(inner.inner))
      i = inner.end
      continue
    }
    current += c
    i += 1
  }
  flush(false)
  return segments.concat(extras)
}

function readWord(source: string, start: number): { value: string; end: number } {
  let i = start
  let quote: "'" | '"' | null = null
  let value = ''
  while (i < source.length) {
    const c = source[i] ?? ''
    if (quote !== null) {
      if (c === quote && (quote === "'" || source[i - 1] !== '\\')) quote = null
      else value += c
      i += 1
      continue
    }
    if (c === "'" || c === '"') {
      quote = c
      i += 1
      continue
    }
    if (/\s/.test(c) || c === ';' || c === '&' || c === '|' || c === '>' || c === '<' || c === '\n') break
    value += c
    i += 1
  }
  return { value, end: i }
}

function readBalanced(source: string, start: number): { inner: string; end: number } {
  let depth = 1
  let quote: "'" | '"' | null = null
  let i = start
  while (i < source.length && depth > 0) {
    const c = source[i] ?? ''
    if (quote !== null) {
      if (c === quote && (quote === "'" || source[i - 1] !== '\\')) quote = null
      i += 1
      continue
    }
    if (c === "'" || c === '"') {
      quote = c
      i += 1
      continue
    }
    if (c === '(') depth += 1
    else if (c === ')') {
      depth -= 1
      if (depth === 0) return { inner: source.slice(start, i), end: i + 1 }
    }
    i += 1
  }
  return { inner: source.slice(start), end: source.length }
}

function readUntil(source: string, start: number, closer: string): { inner: string; end: number } {
  let i = start
  while (i < source.length) {
    if (source[i] === closer && source[i - 1] !== '\\') {
      return { inner: source.slice(start, i), end: i + 1 }
    }
    i += 1
  }
  return { inner: source.slice(start), end: source.length }
}

/** Tokenize one segment, dropping surrounding quotes. */
export function tokenize(text: string): string[] {
  const tokens: string[] = []
  let current = ''
  let quote: "'" | '"' | null = null
  for (let i = 0; i < text.length; i += 1) {
    const c = text[i] ?? ''
    if (quote !== null) {
      if (c === quote && (quote === "'" || text[i - 1] !== '\\')) quote = null
      else current += c
      continue
    }
    if (c === "'" || c === '"') {
      quote = c
      continue
    }
    if (/\s/.test(c)) {
      if (current.length > 0) tokens.push(current)
      current = ''
      continue
    }
    current += c
  }
  if (current.length > 0) tokens.push(current)
  return tokens
}

export interface ProgramInfo {
  program: string
  tokens: string[]
  wrappers: string[]
}

/** Drop `VAR=value` assignments and peel sudo/env/xargs-style wrappers. */
export function programOf(tokens: string[]): ProgramInfo {
  let index = 0
  const wrappers: string[] = []
  const skipAssignments = (): void => {
    while (index < tokens.length && isAssignment(tokens[index] ?? '')) index += 1
  }
  skipAssignments()
  while (index < tokens.length) {
    const raw = tokens[index] ?? ''
    const name = basename(raw).toLowerCase()
    if (!WRAPPERS.has(name)) {
      return { program: name, tokens: tokens.slice(index), wrappers }
    }
    wrappers.push(name)
    index += 1
    while (index < tokens.length && (tokens[index] ?? '').startsWith('-')) {
      const flag = tokens[index] ?? ''
      index += 1
      const flagName = flag.split('=')[0] ?? flag
      if (!flag.includes('=') && WRAPPER_VALUE_FLAGS.has(flagName) && index < tokens.length) {
        index += 1
      }
    }
    if (name === 'timeout' && index < tokens.length && /^\d/.test(tokens[index] ?? '')) {
      index += 1
    }
    skipAssignments()
  }
  return { program: '', tokens: [], wrappers }
}

function isAssignment(token: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*=/.test(token)
}

function basename(path: string): string {
  const slash = path.replace(/\\/g, '/').lastIndexOf('/')
  return slash === -1 ? path : path.slice(slash + 1)
}

function firstNonFlag(tokens: string[]): string {
  for (const token of tokens) {
    if (!token.startsWith('-')) return token.toLowerCase()
  }
  return ''
}

export function isSafeRedirect(target: string): boolean {
  return SAFE_REDIRECTS.has(target)
}

function rule(rules: string[], name: string, detail: string): void {
  const text = `${name} (${detail})`
  if (!rules.includes(text)) rules.push(text)
}

function classifySegment(
  segment: CommandSegment,
  nextProgram: string | undefined,
  rules: string[],
  readOnlyExtra: ReadonlySet<string>,
): { program: string; readonly: boolean } {
  const tokens = tokenize(segment.text)
  const info = programOf(tokens)
  const program = info.program
  const rest = info.tokens.slice(1)
  const joined = info.tokens.join(' ')

  if (info.wrappers.some(w => IDENTITY_WRAPPERS.has(w))) {
    rule(rules, 'permission-identity', info.wrappers.filter(w => IDENTITY_WRAPPERS.has(w)).join('|'))
  }

  if (DESTRUCTIVE_FS.has(program) || /^mkfs/.test(program)) {
    rule(rules, 'destructive-fs', program)
  }
  if (program === 'find' && rest.some(t => t === '-delete' || t === '-exec' || t === '-execdir')) {
    rule(rules, 'destructive-fs', 'find -delete/-exec')
  }
  if (program === 'sed' && rest.some(t => t === '-i' || t.startsWith('-i') || t === '--in-place')) {
    rule(rules, 'destructive-fs', 'sed -i')
  }

  if (PROCESS_CONTROL.has(program)) rule(rules, 'process-control', program)
  if (program === 'systemctl' && rest.some(t => SYSTEMCTL_WRITE.has(t))) {
    rule(rules, 'process-control', `systemctl ${rest.find(t => SYSTEMCTL_WRITE.has(t))}`)
  }
  if (program === 'service' && rest.some(t => SERVICE_WRITE.has(t))) {
    rule(rules, 'process-control', 'service stop/restart')
  }
  if (program === 'supervisorctl') rule(rules, 'process-control', 'supervisorctl')
  if (program === 'pm2' && rest.some(t => PM2_WRITE.has(t))) {
    rule(rules, 'process-control', `pm2 ${rest.find(t => PM2_WRITE.has(t))}`)
  }
  if (program === 'docker' || program === 'docker-compose') {
    const sub = firstNonFlag(rest)
    if (program === 'docker' && (sub === 'compose' || sub === 'stack')) {
      const action = firstNonFlag(rest.slice(rest.findIndex(t => t === sub) + 1))
      if (action === 'down' || DOCKER_WRITE.has(action)) {
        rule(rules, 'process-control', `docker ${sub} ${action}`)
      }
    } else if (DOCKER_WRITE.has(sub)) {
      rule(rules, 'process-control', `${program} ${sub}`)
    }
  }
  if (program === 'kubectl') {
    const sub = firstNonFlag(rest)
    if (sub === 'delete' || sub === 'drain' || sub === 'cordon') {
      rule(rules, 'process-control', `kubectl ${sub}`)
    }
    if (sub === 'rollout' && rest.some(t => t === 'restart')) {
      rule(rules, 'process-control', 'kubectl rollout restart')
    }
  }
  if (program === 'nginx' && rest.includes('-s')) rule(rules, 'process-control', 'nginx -s')

  if (POWER.has(program)) rule(rules, 'power', program)
  if (program === 'init' && (rest[0] === '0' || rest[0] === '6')) rule(rules, 'power', `init ${rest[0]}`)

  if (PERMISSION.has(program)) rule(rules, 'permission-identity', program)

  if (PACKAGE_ALWAYS.has(program)) rule(rules, 'package-mgmt', program)
  if (PACKAGE_SUB.has(program) && PACKAGE_SUB_ACTIONS.has(firstNonFlag(rest))) {
    rule(rules, 'package-mgmt', `${program} ${firstNonFlag(rest)}`)
  }
  if (program === 'gem' && (firstNonFlag(rest) === 'install' || firstNonFlag(rest) === 'uninstall')) {
    rule(rules, 'package-mgmt', `gem ${firstNonFlag(rest)}`)
  }
  if ((program === 'cargo' || program === 'go') && firstNonFlag(rest) === 'install') {
    rule(rules, 'package-mgmt', `${program} install`)
  }

  const nextIsInterpreter = nextProgram !== undefined && (INTERPRETERS.has(nextProgram) || SCRIPT_LANGS.has(nextProgram))
  if (segment.piped && FETCHERS.has(program) && nextIsInterpreter) {
    rule(rules, 'remote-code', `${program} | ${nextProgram}`)
  }
  if (segment.piped && program === 'base64' && rest.some(t => t === '-d' || t === '-D' || t === '--decode') && nextIsInterpreter) {
    rule(rules, 'remote-code', `base64 -d | ${nextProgram}`)
  }
  if (program === 'eval' || program === 'source' || program === '.') {
    rule(rules, 'remote-code', program === '.' ? 'source' : program)
  }

  if (INTERPRETERS.has(program) && rest.some(t => t === '-c' || t.startsWith('-c'))) {
    rule(rules, 'opaque-interpreter', `${program} -c`)
  }
  if (SCRIPT_LANGS.has(program) && rest.some(t => t === '-c' || t === '-e' || t.startsWith('-c') || t.startsWith('-e'))) {
    rule(rules, 'opaque-interpreter', `${program} -c/-e`)
  }

  if (FIREWALL.has(program)) rule(rules, 'firewall', program)
  if (program === 'ip') {
    const obj = firstNonFlag(rest)
    const action = firstNonFlag(rest.filter(t => t !== obj && !t.startsWith('-')))
    if (IP_OBJECTS.has(obj) && IP_WRITES.has(action)) {
      rule(rules, 'firewall', `ip ${obj} ${action}`)
    }
  }

  if (program === 'crontab' && !rest.includes('-l') && !rest.includes('--list')) {
    rule(rules, 'scheduling', 'crontab')
  }
  if (program === 'at' || program === 'atq' || program === 'atrm' || program === 'systemd-run') {
    if (program !== 'atq') rule(rules, 'scheduling', program)
  }

  if (DB_CLIENTS.has(program) && SQL_WRITE.test(joined)) {
    rule(rules, 'db', program)
  }

  if (program === 'git') {
    const sub = firstNonFlag(rest)
    if (sub === 'push' && rest.some(t => t === '--force' || t === '-f' || t.startsWith('--force'))) {
      rule(rules, 'vcs', 'git push --force')
    }
    if (sub === 'reset' && rest.includes('--hard')) rule(rules, 'vcs', 'git reset --hard')
    if (sub === 'clean' && rest.some(t => t === '-f' || t.startsWith('-f') || t === '--force')) {
      rule(rules, 'vcs', 'git clean -f')
    }
    if (sub === 'checkout' && (rest.includes('.') || rest[rest.length - 1] === '.')) {
      rule(rules, 'vcs', 'git checkout .')
    }
    if (sub === 'branch' && rest.some(t => t === '-D')) rule(rules, 'vcs', 'git branch -D')
    if (sub === 'tag' && rest.some(t => t === '-d' || t === '--delete')) rule(rules, 'vcs', 'git tag -d')
    if (sub === 'rebase') rule(rules, 'vcs', 'git rebase')
  }

  for (const target of segment.redirects) {
    if (!isSafeRedirect(target)) rule(rules, 'redirect-write', target)
  }

  const haystack = `${segment.text} ${segment.redirects.join(' ')}`.trim()
  const pathMatch = SENSITIVE_PATH.exec(haystack)
  if (pathMatch) {
    const hit = (pathMatch[1] ?? pathMatch[0]).trim()
    rule(rules, 'sensitive-path', hit)
  }

  return { program, readonly: isReadOnly(info, segment, rest, readOnlyExtra) }
}

function isReadOnly(
  info: ProgramInfo,
  segment: CommandSegment,
  rest: string[],
  readOnlyExtra: ReadonlySet<string>,
): boolean {
  if (segment.redirects.some(target => !isSafeRedirect(target))) return false
  if (info.wrappers.some(w => IDENTITY_WRAPPERS.has(w))) return false
  const program = info.program
  if (program.length === 0) return true
  if (READ_ONLY_PROGRAMS.has(program) || readOnlyExtra.has(program)) return true
  if (program === 'docker') return DOCKER_READ.has(firstNonFlag(rest))
  if (program === 'kubectl') return KUBECTL_READ.has(firstNonFlag(rest))
  if (program === 'systemctl') return SYSTEMCTL_READ.has(firstNonFlag(rest))
  if (program === 'git') return GIT_READ.has(firstNonFlag(rest))
  return false
}

/** Classify a shell command. Danger rules beat the read-only allowlist. */
export function classifyCommand(command: string, options: ClassifyOptions = {}): ClassifyResult {
  const rules: string[] = []
  for (const pattern of options.extraDanger ?? []) {
    if (pattern.test(command)) {
      rules.push(`extra-danger (${pattern.source})`)
    }
  }
  if (rules.length === 0) {
    for (const pattern of options.allow ?? []) {
      if (pattern.test(command)) {
        return { risk: 'safe', rules: ['allow'], programs: [], readonly: true }
      }
    }
  }

  const extra = new Set((options.readOnlyExtra ?? []).map(name => name.toLowerCase()))
  const segments = splitSegments(command)
  const parsed = segments.map(segment => {
    const tokens = tokenize(segment.text)
    return programOf(tokens).program
  })
  const programs: string[] = []
  let readonly = true
  for (let index = 0; index < segments.length; index += 1) {
    const segment = segments[index]
    if (segment === undefined) continue
    const next = segment.piped ? parsed[index + 1] : undefined
    const result = classifySegment(segment, next, rules, extra)
    if (result.program.length > 0) programs.push(result.program)
    if (!result.readonly) readonly = false
  }

  if (rules.length > 0) {
    return { risk: 'danger', rules, programs, readonly: false }
  }
  if (options.production === true && !readonly) {
    return { risk: 'danger', rules: ['production-not-readonly'], programs, readonly: false }
  }
  return { risk: 'safe', rules, programs, readonly }
}

export function normalizeCommand(command: string): string {
  return command.trim().replace(/\s+/g, ' ')
}

function normalizePath(path: string): string {
  return path.trim().replace(/\\/g, '/').replace(/\/{2,}/g, '/')
}

/** Classify a remote path for upload/edit (sensitive) vs download (secret). */
export function classifyRemotePath(path: string): PathKind {
  const normalized = normalizePath(path)
  const lower = normalized.toLowerCase()
  if (
    SECRET_NAME.test(normalized)
    || SECRET_EXT.test(normalized)
    || SECRET_ENV.test(normalized)
    || lower.includes('/.ssh/')
    || lower.endsWith('/.ssh')
    || lower.includes('/.aws/')
    || lower.includes('/.kube/config')
    || lower === '/etc/shadow'
    || lower.endsWith('/shadow')
  ) {
    return 'secret'
  }
  for (const prefix of SENSITIVE_WRITE_PREFIXES) {
    if (lower === prefix || lower.startsWith(`${prefix}/`)) return 'sensitive'
  }
  if (lower.includes('/systemd/') && (lower.endsWith('.service') || lower.endsWith('.timer') || lower.endsWith('.socket'))) {
    return 'sensitive'
  }
  if (lower.includes('/cron.') || lower.endsWith('/crontab')) return 'sensitive'
  return 'ok'
}
