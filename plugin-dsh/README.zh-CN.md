# shellink-dsh

可选的 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) Bundle：把本机 [Shellink](https://github.com/jie123108/Shellink) daemon 暴露成 typed 工具。

插件只做**客户端**。不内嵌 session 引擎、Web UI 或凭据库。卸载时只关本进程的 Unix socket，daemon 上的远程会话继续活着。

## 安全

插件会通过 DeepSeek Harness 的 `ctx.approval` 在下列操作前询问用户：

- 打开会话（`shellink_session_open`，默认每次都问）
- 命中危险分类器的命令（`rm`、`kill`、`chmod`、`systemctl restart`、`curl | sh`、`bash -c`、写 `/etc` 等）；生产环境 profile 上，不在只读白名单里的命令也会问
- 非短应答的 `shellink_session_input`（同一套分类器，避免绕过 exec）
- 创建/更新 `command` profile（在 daemon 宿主机上执行）或生产 SSH host
- 把会话从 `MANUAL` 切回 `AUTO`
- 上传/编辑敏感远程路径，或下载密钥类路径（`.ssh`、`.pem`、`.env`、`/etc/shadow` 等）

`profile_delete` 和 `session_close` 默认不问（`approval.gateProfileDelete` / `approval.gateSessionClose`）。

授权只有一次性的 `allowed-once`。插件按 agent session 在进程内存里记住同一 profile 和完全相同的命令字符串；进程退出即失效，没有磁盘白名单。

分类器是防误操作的启发式，不是安全边界。真正的隔离仍依赖 Shellink profile 的账号权限和 `MANUAL` 模式。请优先使用非生产 profile。

download 会先用 `ctx.fs.resolve` 校验本地路径，再由 daemon 落盘。这只是策略检查，不是 `ctx.fs` 写入那种围栏。

## 安装

需要 `PATH` 上有 `shellink` CLI，以及 DeepSeek Harness `0.1.1-rc.2`（多数 `@deepseek-ai/dsh-*` 包的 `next` dist-tag）。

已发布包：

```sh
dsh plugin --profile web add shellink-dsh
```

本仓库（先 `npm install && npm run build --workspace=plugin-dsh`）。

已安装全局 `dsh`：

```sh
dsh plugin --profile web add .
```

从 DeepSeek Harness **源码仓库**跑时，PATH 上没有 `dsh`，要在那个仓库里用 `pnpm dsh`，并把本包写成绝对路径（`.` 会变成 harness 根目录）。profile 目录是 pnpm workspace，`add` 需要 `-w`：

```sh
cd /path/to/deepseek-harness
pnpm dsh plugin --profile web add -w /path/to/Shellink/plugin-dsh
```

添加 Bundle 后必须重启 profile。确认配置树：

```sh
# 全局 CLI
dsh web --dump-config
# 源码仓库
pnpm dsh web --dump-config
```

应能看到 `id: shellink` 与 `id: shellink-tools`。

## 凭据

创建/更新 profile 的工具只接受 `password_ref` / `private_key_ref` / `passphrase_ref`（POSIX 名），不要传明文。先把值写进 `$DSH_HOME/.credentials.yaml` 或进程环境。明文只出现在发给 daemon 的 RPC 载荷里，不会出现在工具结果中，因此不会进 harness 会话日志。

## 配置

`id: shellink` 的 `config`：

| 键 | 默认 | 含义 |
|---|---|---|
| `socketPath` | `$SHELLINK_SOCKET` 或平台默认 | daemon socket |
| `autoStartDaemon` | `true` | socket 不在时执行 `shellink server start` |
| `daemonReadyTimeoutMs` | `10000` | 拉起后等待就绪 |
| `requestTimeoutMs` | `30000` | 单次 RPC 超时 |
| `waitReadyTimeoutMs` | `30000` | 等待 `AUTO` + `WAITING_INPUT` |
| `execDefaultTimeoutMs` | `20000` | daemon exec 超时 |
| `uploadMaxBytes` | `6291456` | 本地读文件上限 |
| `profileWrites` | `true` | 是否注册 create / update / delete |
| `approval.mode` | `enforce` | `enforce` 走 `ctx.approval`；`off` 关闭审批门 |
| `approval.defaultTier` | `standard` | `production` 把所有主机当生产（白名单模式） |
| `approval.productionPatterns` | `["(?i)prod", "(?i)prd"]` | 匹配 profile 的 name / host / target / uniqueId。前缀 `(?i)` 会转成 JS 的 `i` 标志 |
| `approval.productionProfileIds` | `[]` | 始终视为生产的 profile id |
| `approval.connect` | `always` | `always` / `production-only` / `off` |
| `approval.remember` | `connect+command` | 进程内记忆：`connect+command` / `connect` / `off` |
| `approval.memoMaxEntries` | `200` | 记忆条目的 LRU 上限 |
| `approval.extraDangerPatterns` | `[]` | 额外必审命令正则（优先于 `allowPatterns`） |
| `approval.allowPatterns` | `[]` | 跳过内置分类器的命令正则 |
| `approval.readOnlyExtra` | `[]` | 生产环境额外视为只读的程序名 |
| `approval.gateProfileDelete` | `false` | 删除 profile 前是否询问 |
| `approval.gateSessionClose` | `false` | 关闭 session 前是否询问 |
| `approval.reasonMaxChars` | `400` | 展示给用户的 reason 截断长度 |

设 `profileWrites: false` 则只保留 list/get。`approval.*Patterns` 里的非法正则会在 tools 插件加载时抛错。

## 工具

`shellink_profile_*`、`shellink_session_*`（list / open / state / history / input / exec / mode / close）、以及 upload / download / edit。

长任务用 `run_in_background`，再用 harness 自带的 `job_output` / `job_kill` 收割或取消。
