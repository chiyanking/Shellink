import { cpSync, mkdirSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const root = resolve(import.meta.dirname, '..')
const run = (command, args, options = {}) => {
  const result = spawnSync(command, args, { cwd: root, stdio: 'inherit', ...options })
  if (result.error) throw result.error
  if (result.status !== 0) process.exit(result.status ?? 1)
}

run(process.execPath, [process.env.npm_execpath, 'run', 'build', '--workspace=web'])
cpSync(join(root, 'web', 'dist'), join(root, 'internal', 'httpapi', 'assets'), { recursive: true })
writeFileSync(join(root, 'internal', 'httpapi', 'assets', 'complete.txt'), 'ready\n')
mkdirSync(join(root, 'dist'), { recursive: true })

const args = process.argv.slice(2)
const all = args.includes('--all')
const targetIndex = args.indexOf('--target')
const hostOS = { win32: 'windows', darwin: 'darwin', linux: 'linux' }[process.platform]
const hostArch = { x64: 'amd64', arm64: 'arm64' }[process.arch]
const targets = all
  ? [
      ['darwin', 'arm64'], ['darwin', 'amd64'], ['linux', 'arm64'],
      ['linux', 'amd64'], ['windows', 'arm64'], ['windows', 'amd64'],
    ]
  : targetIndex >= 0
    ? [[args[targetIndex + 1], args[targetIndex + 2]]]
    : [[process.env.GOOS || hostOS, process.env.GOARCH || hostArch]]

if (targets.some(([goos, goarch]) => !goos || !goarch)) {
  throw new Error('unable to resolve Go build target')
}

for (const [goos, goarch] of targets) {
  const crossTarget = all || targetIndex >= 0
  const arch = goarch === 'amd64' ? 'x64' : goarch
  const name = crossTarget
    ? `shellink-${goos}-${arch}${goos === 'windows' ? '.exe' : ''}`
    : `shellink${goos === 'windows' ? '.exe' : ''}`
  run('go', ['build', '-o', join(root, 'dist', name), './cmd/shellink'], {
    cwd: root,
    env: { ...process.env, GOOS: goos, GOARCH: goarch },
  })
}
