// Transitional build only. This Go core is removed after the Rust Core
// migration; keeping the hook now lets the moved root Desktop remain usable.
import { mkdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const root = resolve(import.meta.dirname, '..')
const hostArch = { arm64: 'aarch64', x64: 'x86_64' }[process.arch]
const hostOS = { darwin: 'apple-darwin', linux: 'unknown-linux-gnu', win32: 'pc-windows-msvc' }[process.platform]
const target = process.env.TAURI_ENV_TARGET_TRIPLE || process.env.CARGO_BUILD_TARGET || `${hostArch}-${hostOS}`
const goos = target.includes('windows') ? 'windows' : target.includes('darwin') ? 'darwin' : target.includes('linux') ? 'linux' : null
const goarch = target.startsWith('aarch64') ? 'arm64' : target.startsWith('x86_64') ? 'amd64' : null
if (!goos || !goarch) throw new Error(`Unsupported transition core target: ${target}`)
const dir = join(root, 'src-tauri/binaries')
mkdirSync(dir, { recursive: true })
const output = join(dir, `shellink-core-${target}${goos === 'windows' ? '.exe' : ''}`)
const result = spawnSync('go', ['build', '-trimpath', '-o', output, './cmd/shellink'], { cwd: root, env: { ...process.env, GOOS: goos, GOARCH: goarch }, stdio: 'inherit' })
if (result.error) throw result.error
process.exit(result.status ?? 1)
