---
name: shellink-cli
description: >-
  Use when operating remote SSH or command sessions, running remote commands,
  interactive terminals, file transfer, or remote file edits through the local
  Shellink binary. Never implement the Unix socket protocol directly.
  （当需要操作远程 SSH 或命令会话、执行远程命令、使用交互式终端、传输文件或编辑远程文件，且本地存在 Shellink 二进制时使用。切勿自行实现 Unix socket 协议。）
---

# Shellink CLI

通过本地 `shellink` 二进制驱动远程 SSH 和命令会话。
切勿自行实现或直连 Unix socket / MessagePack 协议。

## 定位二进制

二进制不在 PATH 上，实际位于：

    /Users/wangtk/Desktop/tool/shellink-cli/shellink

每次会话解析一次并复用：

```bash
SL="$(command -v shellink || echo "$HOME/Desktop/tool/shellink-cli/shellink")"
"$SL" -V   # 验证输出："Shellink <版本>  commit <commit>  ·  protocol <n>"
```

两者都失败时，告知用户二进制缺失。不要下载或安装任何东西。

下文命令写作 `shellink ...`，实际执行时用 `"$SL" ...`。所有命令都会自动启动用户级守护进程。加 `--json` 获取稳定的机器可读输出。

## 输出格式（重要）

默认输出是**给人看的**：列表渲染成对齐表格，对象渲染成对齐的 key/value 行，
状态、模式、布尔值、URL、体积各自带颜色；命令输出以原样块的形式打印在对应 key 下。
给 AI/Agent 或脚本解析时必须加 `--json`，此时输出是**单行紧凑 JSON**，与以前完全一致。

```bash
"$SL" session list            # 人看：表格 + 状态配色
"$SL" session list --json     # 脚本：单行 JSON
"$SL" profile get <id>        # 人看：对齐字段
"$SL" --color never session list   # 不要颜色（也可设 NO_COLOR=1）
```

表格放不下的列会被收起，并在末尾提示 `… N more columns · use --json for everything`，
即完整数据只在 `--json` 里。

### 会话历史的颜色

`session history` 与网页端「历史审查」页会还原远端 shell 自己产生的 ANSI 颜色，
并记录下来的提示符（`user@host:/path#`）— 身份绿、路径蓝、root 的 `#` 黄，
便于一眼分辨“我在哪台机器”与“这是程序输出还是命令”。
脚本/AI 消费时加 `--json`（或请求不带 `color`），拿到的仍是无转义的纯文本。

## 任何 Shellink 任务的第一步

先读安装的命令参考——它是权威文档，可能比本 skill 更新：

```bash
shellink agent-doc
```

## 工作流

```bash
shellink profile list --query '<名称、主机、IP 或命令关键字>' --json
shellink session create --profile '<profile-id>' --json
shellink session state '<session-id>' --json
shellink session exec '<session-id>' --command 'uname -a' --json
shellink session history '<session-id>' --since 0 --json
shellink session input '<session-id>' --text 'yes' --json
shellink session close '<session-id>' --json
```

会话状态：CONNECTING、OUTPUTTING、WAITING_INPUT、IDLE、DISCONNECTED。

- exec 前先等状态到 WAITING_INPUT。
- IDLE 时先查 history；仅当程序明显在等待输入时才用 `input`。
- MANUAL 会话由人控制，不要强行 input；等待其变为 AUTO（或显式切换：`shellink session mode '<id>' --mode AUTO`）。
- 任务完成后保持会话开启以便复用。仅当用户明确要求时才关闭会话。

创建会话前先用 `--query` 搜索 profile——绝不要列出全部 profile。凭据通过 `--input FILE` 或 `--input -` 传入 JSON，
绝不放在命令行参数里。

## 长耗时操作

宿主 agent 对每条 shell 命令只等待约 30 秒，之后会转后台。默认超时
（exec 20s、edit 25s、transfer 120s）都控制在该窗口内。可能更慢的操作一律加 `--detach`：

```bash
shellink session exec '<session-id>' --command 'long-build' --detach --json
shellink session exec-status '<job-id>' --since <cursor> --wait 20000 --json
# 重复调用直到 done 为 true；每次传入上一次返回的 cursor
shellink session exec-cancel '<job-id>' --json   # 发送 Ctrl+C 中止
```

- `exec-status` 最多长轮询 `--wait` 毫秒——每次调用都控制在 30 秒内，
  同时保持推进。
- `upload`、`download`、`edit` 也支持 `--detach`；用 `session exec-status`
  轮询返回的 job。
- `download ... --detach --output /local/path`——由守护进程写文件；
  job 的 `output` 字段会确认路径。
- 409 "session is running a ..."——消息里指明了正在运行的 job。对它轮询
  `exec-status`，或用 `exec-cancel` 取消；不要盲目重试。
- exec 返回 `timedOut: true`——远程命令仍在运行并持有会话锁。用返回的
  `jobId`/`cursor` 轮询 `exec-status`，或取消该 job。
- 超长远程任务：`nohup cmd > /tmp/x.log 2>&1 &` 分离执行，然后用
  `session exec --command 'tail -n 50 /tmp/x.log'` 轮询日志。
- `METHOD_NOT_FOUND`——守护进程版本低于 CLI；执行 `shellink server restart`。

## 文件

```bash
shellink session download '<id>' --path /remote/file --output ./file --json
shellink session upload '<id>' --input ./file --path /remote/file --json
shellink session edit '<id>' --input edits.json --json
```

`edits.json` 包含远程路径和精确替换内容——每条 `oldText` 必须非空、
在文件中恰好匹配一次（含空白字符）；多条编辑不得重叠。远程编辑要求会话
为 AUTO 且处于 WAITING_INPUT：

```json
{"path":"/remote/file","edits":[{"oldText":"exact text","newText":"replacement"}]}
```

## 退出码

0 成功 · 1 运行时/服务错误 · 2 参数或校验错误。

## 升级（可选）

仅当用户要求时：

```bash
shellink upgrade --check --json
shellink upgrade --yes --json
```
