# Shellink 桌面界面

Shellink 的桌面应用位于仓库根目录，使用 Vite / React / Tauri v2。Rust Core 与窗口同生命周期；当前 Go bridge 仅作为迁移过渡。

使用 `create-tauri-ui@1.2.0` 生成基础工程，再迁入 Shellink 界面。保留窗口居中、加载后显示、桌面滚动和文本选择约定；移除模板问候命令、调试面板和外链插件。依赖直接由仓库根目录的 npm 管理，无需安装 Bun。

## 预览

在仓库根目录执行：

```sh
npm install
npm run dev
```

浏览器开发服务器只用于前端样式开发；业务 RPC 需要在 Tauri 原生窗口中运行，并由随包核心连接后台。

```sh
npm run build   # 类型检查和前端产物 dist
npm run desktop:dev     # Tauri 原生桌面窗口
npm run desktop:build   # 当前系统的原生应用包
```

原生窗口及打包需要 Rust 和对应平台的 [Tauri 系统依赖](https://tauri.app/start/prerequisites/)。原生构建产物位于 `src-tauri/target/release/bundle/`；Windows 的 exe 需要在 Windows 环境构建。迁移完成后应用包只包含 Rust Core，不再包含 Go。当前 Go bridge 退出时会先请求停止旧 runtime，确保 MCP HTTP 和终端会话一起结束。详见 [整体架构](architecture.md)。

## 当前范围

桌面已接入连接配置、session 生命周期与模式切换、终端回放和实时输出、MCP 配置、Webhook 消息及真实终端历史。历史页展示后台记录的原始输入与合并 PTY 输出；来源身份未知，不拆分 stdout/stderr，也不推断退出码。

- 工作台：真实 xterm 会话回放、实时输出、MANUAL 输入、尺寸同步、标签与分屏；当前后台未提供目录浏览接口。
- 会话管理：同一连接的多个 session、来源、状态、详情、重命名和结束。
- 连接管理：连接列表、搜索、新建、编辑、删除和 JSON 导入。
- AI 接入：显示后台生成的 CLI / MCP 配置、真实 HTTP 状态与 Token 轮换。
- 历史记录：按 session 分页显示原始终端片段并导出 JSON；不声称具备结构化操作审计。
- 设置：深浅主题、终端字体和本机偏好；显示真实后台诊断并打开数据目录。
- Webhook：真实接收地址、消息列表、筛选、复制、刷新和清空。

`Cmd/Ctrl+K` 打开快速访问，`Cmd/Ctrl+1–5` 切换页面（`2` 为会话管理），`Shift+Escape` 切换当前 session 控制权。会话与连接业务数据由后台持久化；主题、字体和标签偏好保存在本机浏览器存储。

Webhook 消息与终端历史分开展示，消息携带 sessionId 时提供关联入口。创建入口来源只标为已知的 Desktop、CLI、MCP 或未知；这不等同于操作审计中的操作者身份。

会话的集中入口是左侧「会话管理」。工作台的「全部会话」、单个终端的「管理此会话」和底栏的活跃会话数也可进入。收起终端标签只关闭视图；结束会话的独立确认弹窗展示进程终止与保留记录的设计语义。详见 [会话管理设计](design/session-management.md)。

仍未实现：完整结构化操作审计、远端目录浏览/传输 UI、登录自启动、PATH 安装、托盘与“停止后台”界面。创建的新 session 初始为 MANUAL，用户可切换 AUTO；终端键盘写入仅在 MANUAL 下发送。旧版 daemon 若不支持 `desktop.info`，需先由用户正常停止该 daemon，再用新版核心启动，不会自动终止旧会话进程。

## 文件结构

```text

  src/
    App.tsx              窗口内容、导航、弹窗、后台快照
    Workspace.tsx        终端工作区与活动侧栏
    Sessions.tsx         会话管理、详情与生命周期操作
    Pages.tsx            连接、AI 接入、设置
    History.tsx          按 session 的操作记录与输入输出详情
    Webhook.tsx          收到的回调消息与 JSON 原文
    audit-data.ts        旧审计示意数据定义，不用于运行界面
    review.css           历史记录与 Webhook 界面样式
    components/ui/       基于 Radix 的本地 shadcn 风格组件
    data.ts              后台 DTO 到界面模型的映射
    desktop.css          主题、布局、响应式样式
  src-tauri/             Tauri 窗口与打包配置
  app-icon.svg           应用图标源文件
```
