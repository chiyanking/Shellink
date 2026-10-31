# Desktop 开发

Shellink Desktop 位于仓库根目录，使用 Vite、React 和 Tauri v2。业务核心由 `src-tauri/src/` 中的 Rust 代码提供，Tauri 窗口、Rust 数据库、终端子进程和 MCP HTTP 监听器属于同一个生命周期。

## 常用命令

```sh
npm ci
npm run desktop:dev
npm run desktop:build
npm run desktop:build:debug
npm run build
cargo check --manifest-path src-tauri/Cargo.toml
cargo test --manifest-path src-tauri/Cargo.toml
```

`npm run dev` 只启动 Vite。原生窗口开发和打包使用 `desktop:*` 命令，Tauri 会自动执行前端构建命令。

## 目录

- `src/`：页面、组件、样式和 Tauri RPC 客户端。
- `src-tauri/src/bridge.rs`：Tauri command 与 Desktop RPC 分发。
- `src-tauri/src/sessions.rs`：本地 command/SSH 会话、输入、输出、历史和退出回收。
- `src-tauri/src/data.rs`：SQLite schema、加密 profile、MCP 配置和历史记录。
- `src-tauri/src/mcp.rs`：MCP HTTP 服务、Webhook 回调和 stdio 转发。
- `src-tauri/tauri.conf.json`：窗口、前端地址和打包设置。

## 生命周期

Rust core 在 Tauri setup 阶段初始化数据库并启动 MCP HTTP。窗口关闭会触发 Tauri exit；退出处理器先停止 HTTP listener，再关闭所有活动 session 子进程，最后释放事件订阅。stdio 客户端通过当前 Desktop 的 HTTP listener 工作，因此 Desktop 退出后不会留下独立的终端或 MCP 服务。

## 配置

`SHELLINK_HOME` 可指定数据目录，`SHELLINK_DB` 可指定 SQLite 文件，`SHELLINK_MASTER_KEY` 可提供 32 字节十六进制主密钥。MCP HTTP 使用 `SHELLINK_HOST`、`SHELLINK_PORT` 和 `SHELLINK_HTTP_ENABLED`。Profile 中的 password、private key 和 passphrase 只以加密形式写入数据库。

## RPC 与事件

前端通过 `core_request` 调用 Rust RPC，不直接访问 SQLite 或 HTTP。主要方法包括：

- `desktop.info`、`system.hello`、`system.ping`、`system.status`、`system.stop`。
- `profiles.list/get/create/update/delete`。
- `sessions.list/create/state/history/historyChunks/replay/input/mode/resize/close/removeRecord/rename`。
- `mcp.info`、`mcp.config` 和 `webhooks.list/create/delete/messages/clearMessages`。
- `events.subscribe`、`events.unsubscribe`。

事件通过 Tauri `shellink:event` 发送。订阅可以指定 `sessionId`，Desktop 只向匹配的订阅发送 session 事件。
