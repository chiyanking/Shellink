// Builds the folder that is handed out: a copy that runs from anywhere and
// keeps its own data inside the folder.
//
// Everything Shellink needs is already inside the binary (pure Go, embedded
// database, embedded web UI), so "works elsewhere" is only about two things:
// the two executables have to sit in the same folder, and the data directory
// has to follow that folder. The latter is what the portable marker does, so
// the archive is self-contained and can also be moved to a USB stick.
import { spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const arch = 'x64'
const platform = 'windows'

const core = join(root, 'dist', `shellink-${platform}-${arch}.exe`)
if (!existsSync(core)) {
  console.error(`缺少 ${core}，请先执行: npm run build:binary:${platform}-${arch}`)
  process.exit(1)
}

const run = (command, args, options = {}) => {
  const result = spawnSync(command, args, { cwd: root, stdio: 'inherit', ...options })
  if (result.error) throw result.error
  if (result.status !== 0) process.exit(result.status ?? 1)
}

const staging = join(root, 'dist', 'Shellink-Windows-x64')
const zip = join(root, 'dist', 'Shellink-Windows-x64.zip')
rmSync(staging, { recursive: true, force: true })
rmSync(zip, { force: true })
mkdirSync(staging, { recursive: true })

// The launcher is a separate module, so it is built here rather than by
// build-go.mjs.
run('go', ['build', '-trimpath', '-ldflags', '-s -w', '-o', join(staging, 'Shellink启动器.exe'), '.'], {
  cwd: join(root, 'launcher'),
  env: { ...process.env, GOOS: platform, GOARCH: 'amd64', CGO_ENABLED: '0' },
})
copyFileSync(core, join(staging, 'shellink.exe'))

// Presence of this file is what keeps the data in ./shellink-data. Its content
// is never read, which makes deleting it the documented way to switch a folder
// back to per-user storage.
writeFileSync(
  join(staging, 'shellink.portable'),
  [
    '# 删除本文件 → 数据改回保存在 %APPDATA%\\Shellink',
    '# 保留本文件 → 数据保存在程序目录下的 shellink-data，整个文件夹可以搬走',
    '',
  ].join('\n')
)

writeFileSync(
  join(staging, '使用说明.txt'),
  readme(),
  // Notepad guesses the encoding wrong often enough to warrant a BOM.
  { encoding: 'utf8' }
)

run('powershell', [
  '-NoProfile',
  '-Command',
  `Compress-Archive -Path '${staging}' -DestinationPath '${zip}' -CompressionLevel Optimal -Force`,
])

console.log(`\n打包完成: ${zip}  (${(statSync(zip).size / 1024 / 1024).toFixed(1)} MB)`)
console.log('把整个 shellink 文件夹或 zip 发给别人即可，解压到任意目录、双击“Shellink启动器.exe”运行。')

function readme() {
  return `﻿Shellink 使用说明
================

一、怎么运行
----------
1. 解压到任意目录（不要直接在压缩包里双击运行）。
2. 双击 "Shellink启动器.exe"，程序会自动打开浏览器进入控制台。
3. 关闭这个黑窗口不会影响服务；要停止服务请在同一目录执行:
   shellink.exe server stop
   或在本窗口查看其它常用命令。

二、运行依赖（很少）
----------
完全不需要安装 Node.js、Go、数据库或其他运行库，两个 exe 已经包含全部内容。

必须项:
  · Windows 10 1809（内部版本 17763）或更高 / x64
    —— 命令终端依赖系统的 ConPTY，更早的版本只能使用 SSH 会话。
  · OpenSSH 客户端（仅 SSH 会话需要）
    Windows 10/11 自带但可能没启用。启用方法任选其一:
    · 设置 → 系统 → 可选功能 → 添加功能 → 勾选 "OpenSSH 客户端"
    · 管理员 PowerShell 执行: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
    缺失不影响启动和本地命令会话，只是建不了 SSH 会话。

不需要:
  · 不需要 Node.js / Go / SQLite / VC++ 运行库（均为静态编译，Web 界面已内置）
  · 不需要联网才能使用（仅 SSH 会话需要连得上目标服务器）
  · 不需要管理员权限（建议直接双击，不要用“以管理员身份运行”）

三、数据保存在哪里
----------
本目录里有 shellink.portable 文件，所以数据保存在本文件夹的 shellink-data 里:
  · 会话、主机配置、密钥 → shellink-data\\shellink.db（密钥用 master.key 加密）
  · 运行日志           → shellink-data\\shellink.log
  · 整个文件夹可以复制到 U 盘或别的电脑，历史记录跟着走。

想改回存到当前用户（%APPDATA%\\Shellink），删除 shellink.portable 即可；
想指定其它位置，设置环境变量 SHELLINK_HOME 指向一个可写目录。

四、环境问题怎么查
----------
在本目录执行:
  Shellink启动器.exe --check
会列出操作系统、ConPTY、SSH 客户端、数据目录、端口 7070、磁盘空间、权限的检查结果，
缺什么就提示怎么装。正常启动时只会在真的有问题时才提示。

出问题还会生成诊断文件:
  shellink-launcher-error.txt（在本目录下，包含完整错误线索）

五、常见问题
----------
· 端口 7070 被占用 —— 启动器会先自动结束旧的 shellink.exe，仍失败则多为别的程序占用。
· 数据目录不可写 —— 把该目录加入安全软件信任列表，或设置 SHELLINK_HOME 指到可写目录。
· 提示“低完整性(Low)” —— 程序文件夹被 Windows 标成低完整性（下载/解压的压缩包常见）。
  管理员权限下执行:
    icacls "本文件夹完整路径" /setintegritylevel (OI)(CI)Medium /T
· 浏览器弹“现有实例正在以提升的权限运行” —— 关掉那个管理员权限的浏览器窗口再打开。
`.replace(/\n/g, '\r\n')
}
