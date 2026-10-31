import { useState } from 'react'
import { rpc, openDirectory } from './backend'
import {
  ArrowRight,
  ArrowUpRight,
  Braces,
  Check,
  CircleHelp,
  Code2,
  Copy,
  FileClock,
  FolderOpen,
  Globe,
  Hand,
  ListFilter,
  Monitor,
  Moon,
  MoreHorizontal,
  Plus,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  Sun,
  Terminal,
  Trash2,
  Upload,
  Workflow,
  Webhook,
  Zap,
} from 'lucide-react'
import WebhookMessages from './Webhook'
import { useDesign } from './context'
import { Badge, Button, Input, Menu, Select, Switch } from './components/ui'
import { cn } from './lib/utils'
import { PageHeading } from './Workspace'

export function ConnectionsPage() {
  const { connections, openConnection, openModal, notify, sessions, inspectSession, editConnection, deleteConnection } = useDesign()
  const [filter, setFilter] = useState('all')
  const [search, setSearch] = useState('')
  const shown = connections.filter(
    (item) =>
      (filter === 'all' || item.type === filter) &&
      `${item.name} ${item.host} ${item.group}`.toLowerCase().includes(search.toLowerCase()),
  )
  return (
    <div className="standard-page connections-page">
      <PageHeading
        eyebrow="CONNECTIONS"
        title="连接管理"
        description="管理主机与认证配置。一个连接，可以创建多个独立会话。"
      >
        <Button variant="outline" onClick={() => openModal('import')}>
          <Upload size={14} />
          导入连接
        </Button>
        <Button onClick={() => openModal('connection')}>
          <Plus size={15} />
          新建连接
        </Button>
      </PageHeading>
      <div className="connections-toolbar">
        <div className="filter-tabs" aria-label="连接类型">
          {[
            { id: 'all', label: '全部连接', count: connections.length },
            { id: 'SSH', label: 'SSH', count: connections.filter(item => item.type === 'SSH').length },
            { id: 'Local', label: '本地', count: connections.filter(item => item.type === 'Local').length },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => setFilter(tab.id)}
              aria-pressed={filter === tab.id}
              className={filter === tab.id ? 'is-active' : ''}
            >
              {tab.label}
              <span>{tab.count}</span>
            </button>
          ))}
        </div>
        <div className="toolbar-right">
          <label className="search-input">
            <Search size={14} />
            <Input
              placeholder="搜索名称、地址或分组…"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              aria-label="搜索连接"
            />
          </label>
          <Menu
            trigger={
              <Button variant="outline" size="icon" aria-label="连接筛选">
                <ListFilter size={15} />
              </Button>
            }
            items={[
              { label: '全部分组', checked: true },
              { label: '按名称排序', onSelect: () => notify('当前连接按分组排列。') },
            ]}
          />
        </div>
      </div>
      <div className="connection-table-wrap">
        <table className="connection-table">
          <thead>
            <tr>
              <th>连接名称</th>
              <th>目标地址</th>
              <th>分组</th>
              <th>活跃会话</th>
              <th>最近使用</th>
              <th>
                <span className="sr-only">操作</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {shown.map((connection) => {
              const activeSessions = sessions.filter(
                (session) => session.connectionId === connection.id && session.status !== 'ended',
              )
              return (
                <tr key={connection.id}>
                  <td>
                    <button
                      className="table-connection"
                      onClick={() => openConnection(connection.id)}
                    >
                      <span className={cn('connection-icon', `color-${connection.color}`)}>
                        {connection.type === 'SSH' ? <Server size={17} /> : <Terminal size={17} />}
                      </span>
                      <span>
                        <strong>{connection.name}</strong>
                        <small>
                          {connection.type === 'SSH' ? 'SSH · 端口 22' : '本地 Shell · zsh'}
                        </small>
                      </span>
                    </button>
                  </td>
                  <td className="mono table-address">
                    {connection.user}@{connection.host}
                  </td>
                  <td>
                    <Badge>{connection.group}</Badge>
                  </td>
                  <td>
                    <button
                      className={cn(
                        'connection-state connection-session-count',
                        !activeSessions.length && 'is-offline',
                      )}
                      disabled={!activeSessions.length}
                      onClick={() => inspectSession(activeSessions[0]!.id)}
                    >
                      <span className="status-dot" />
                      {activeSessions.length ? `${activeSessions.length} 个会话` : '暂无会话'}
                      {activeSessions.length > 0 && <ArrowUpRight size={11} />}
                    </button>
                  </td>
                  <td className="subtle">{activeSessions.length ? '刚刚' : '昨天'}</td>
                  <td>
                    <div className="table-actions">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => openConnection(connection.id)}
                      >
                        {activeSessions.length ? '打开会话' : '新建会话'}
                        <ArrowUpRight size={13} />
                      </Button>
                      <Menu
                        trigger={
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`${connection.name} 的更多操作`}
                          >
                            <MoreHorizontal size={16} />
                          </Button>
                        }
                        items={[
                          {
                            label: '编辑连接',
                            icon: <Settings2 size={14} />,
                            onSelect: () => editConnection(connection.id),
                          },
                          {
                            label: '复制目标地址',
                            icon: <Copy size={14} />,
                            onSelect: () => void navigator.clipboard.writeText(connection.type === 'SSH' ? `${connection.user}@${connection.host}:${connection.port}` : connection.command ?? '').then(() => notify('已复制目标地址')).catch(() => notify('复制失败')),
                          },
                          {
                            label: '查看历史记录',
                            icon: <FileClock size={14} />,
                            onSelect: () => { const session = sessions.find(item => item.connectionId === connection.id); if (session) inspectSession(session.id); else notify('此连接尚无会话') },
                          },
                          { label: '删除连接配置', icon: <Trash2 size={14} />, onSelect: () => { if (window.confirm(`删除连接“${connection.name}”？现有会话和历史会保留。`)) void deleteConnection(connection.id) } },
                        ]}
                      />
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {!shown.length && (
          <div className="empty-state">
            <Search size={25} />
            <h3>没有匹配的连接</h3>
            <p>试试其他名称、地址或分组。</p>
            <Button
              variant="outline"
              onClick={() => {
                setSearch('')
                setFilter('all')
              }}
            >
              清除筛选
            </Button>
          </div>
        )}
      </div>
      <div className="table-footer">
        <span>共 {shown.length} 个连接</span>
        <span>
          <ShieldCheck size={13} />
          连接信息仅保存在本地
        </span>
      </div>
      <section className="import-banner">
        <div className="import-banner-icon">
          <FolderOpen size={22} />
        </div>
        <div>
          <h3>把熟悉的连接带过来</h3>
          <p>导入 Shellink 连接 JSON，继续使用已有的主机配置。</p>
        </div>
        <Button variant="outline" onClick={() => openModal('import')}>
          导入配置
          <ArrowRight size={14} />
        </Button>
      </section>
    </div>
  )
}

export function IntegrationsPage() {
  const { notify, mcp, refresh } = useDesign()
  const [method, setMethod] = useState('mcp')
  const [client, setClient] = useState('Claude Code')
  const [transport, setTransport] = useState('stdio')
  const [httpEnabled, setHttpEnabled] = useState(mcp?.httpEnabled ?? false)
  const [busy, setBusy] = useState(false)
  const executable = mcp?.command ?? ''
  const config =
    transport === 'http'
      ? JSON.stringify(
          {
            mcpServers: {
              shellink: {
                url: mcp?.url ?? '',
                headers: { Authorization: `Bearer ${mcp?.token ?? ''}` },
              },
            },
          },
          null,
          2,
        )
      : client === 'Codex'
        ? `[mcp_servers.shellink]\ncommand = ${JSON.stringify(executable)}\nargs = ["mcp"]`
        : JSON.stringify(
            {
              mcpServers: {
                shellink: {
                  command: executable,
                  args: ['mcp'],
                },
              },
            },
            null,
            2,
          )
  const cli =
    'shellink session list --json\n\nshellink session exec SESSION_ID \\\n  --command "docker compose ps" \\\n  --json'
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      notify('已复制后台生成的配置')
    } catch {
      notify('请从配置区域选中并复制文本。')
    }
  }
  return (
    <div className="standard-page integrations-page">
      <PageHeading
        eyebrow="AI INTEGRATIONS"
        title="AI 接入"
        description="让你喜欢的 AI，使用你熟悉的终端。"
      >
        <Badge className="preview-badge">
          <span className="status-dot" />
          配置预览
        </Badge>
      </PageHeading>
      <div className="integration-methods" aria-label="接入方式">
        <button
          className={method === 'mcp' ? 'is-active' : ''}
          aria-pressed={method === 'mcp'}
          onClick={() => setMethod('mcp')}
        >
          <Workflow size={16} />
          <span>MCP 接入</span>
          <Badge tone="blue">推荐</Badge>
        </button>
        <button
          className={method === 'cli' ? 'is-active' : ''}
          aria-pressed={method === 'cli'}
          onClick={() => setMethod('cli')}
        >
          <Terminal size={16} />
          <span>CLI 命令行</span>
        </button>
      </div>
      <div className="integration-layout">
        <section className="integration-setup">
          <div className="panel-title">
            <span className="step-number">1</span>
            <div>
              <h2>{method === 'mcp' ? '选择你的 AI 客户端' : '从命令行开始'}</h2>
              <p>
                {method === 'mcp'
                  ? '复制配置，添加到客户端的 MCP 设置。'
                  : '在现有 AI 工作流中使用结构化命令。'}
              </p>
            </div>
          </div>
          {method === 'mcp' ? (
            <>
              <div className="client-choices">
                {[
                  { name: 'Claude Code', symbol: '✳', className: 'client-claude' },
                  { name: 'Codex', symbol: '⌘', className: 'client-codex' },
                  { name: 'Cursor', symbol: '◈', className: 'client-cursor' },
                ].map((item) => (
                  <button
                    key={item.name}
                    className={cn('client-choice', client === item.name && 'is-active')}
                    onClick={() => setClient(item.name)}
                    aria-pressed={client === item.name}
                  >
                    <span className={item.className} aria-hidden="true">
                      {item.symbol}
                    </span>
                    <strong>{item.name}</strong>
                    {client === item.name && <Check size={13} />}
                  </button>
                ))}
              </div>
              <div className="transport-row">
                <span>传输方式</span>
                <div className="small-segments">
                  <button
                    className={transport === 'stdio' ? 'is-active' : ''}
                    aria-pressed={transport === 'stdio'}
                    onClick={() => setTransport('stdio')}
                  >
                    stdio
                  </button>
                  <button
                    className={transport === 'http' ? 'is-active' : ''}
                    aria-pressed={transport === 'http'}
                    onClick={() => setTransport('http')}
                  >
                    HTTP
                  </button>
                </div>
              </div>
            </>
          ) : (
            <div className="cli-path">
              <Terminal size={15} />
              <code>{executable || '后台路径不可用'}</code>
            </div>
          )}
          <div className="config-code">
            <div className="config-code-heading">
              <span>
                <Braces size={13} />
                {method === 'cli'
                  ? 'Terminal'
                  : client === 'Codex' && transport === 'stdio'
                    ? 'config.toml'
                    : 'mcp.json'}
                <Badge>本机配置</Badge>
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => copy(method === 'cli' ? cli : config)}
              >
                <Copy size={13} />
                复制
              </Button>
            </div>
            <pre className="ui-selectable">
              <code>{method === 'cli' ? cli : config}</code>
            </pre>
          </div>
          <div className="integration-note">
            <CircleHelp size={14} />
            <span>配置由当前桌面后台生成。</span>
          </div>
          <div className="integration-next-step">
            <span className="step-number">2</span>
            <div>
              <h3>{method === 'mcp' ? '保存配置，重新连接客户端' : '交给 AI，继续你的工作'}</h3>
              <p>
                {method === 'mcp'
                  ? '接入后，AI 可发现已有主机并复用持久终端会话。'
                  : '命令返回 JSON 结果，适合自动化工具读取和处理。'}
              </p>
            </div>
          </div>
        </section>
        <aside className="integration-aside">
          <section className="capabilities-panel">
            <div className="section-label">
              <Zap size={14} />
              为终端协作准备
            </div>
            <h3>一个会话，多个入口。</h3>
            <p>界面、CLI 和 MCP 共享同一份终端状态。</p>
            <div className="capability">
              <Server size={16} />
              <div>
                <strong>连接与会话</strong>
                <span>发现主机，复用正在运行的终端</span>
              </div>
            </div>
            <div className="capability">
              <Terminal size={16} />
              <div>
                <strong>命令与任务</strong>
                <span>提交命令，持续观察执行输出</span>
              </div>
            </div>
            <div className="capability">
              <Hand size={16} />
              <div>
                <strong>随时人工接管</strong>
                <span>暂停 AI 输入，保持操作可控</span>
              </div>
            </div>
            <div className="capability">
              <FileClock size={16} />
              <div>
                <strong>统一操作记录</strong>
                <span>知道谁在何时执行了什么</span>
              </div>
            </div>
            <div className="capability-footnote">以上展示产品设计目标</div>
          </section>
          <section className="http-panel">
            <div>
              <Globe size={15} />
              <strong>MCP HTTP</strong>
              <Switch
                checked={mcp?.httpEnabled ?? httpEnabled}
                disabled={busy || !mcp}
                onCheckedChange={async value => { setBusy(true); try { await rpc('mcp.config', {enabled:value}); setHttpEnabled(value); await refresh() } catch(error) { notify(String(error)) } finally { setBusy(false) } }}
                aria-label="启用 MCP HTTP"
              />
            </div>
            <p>{mcp?.httpEnabled ? '本机 HTTP · 独立令牌认证' : 'HTTP MCP 已关闭'}</p>
            <span>{mcp?.url ?? '服务地址不可用'}</span>
            {mcp?.httpEnabled && <Button variant="outline" size="sm" disabled={busy} onClick={async () => { setBusy(true); try { await rpc('mcp.config',{regenerateToken:true}); await refresh(); notify('已轮换 MCP Token') } catch(error) { notify(String(error)) } finally { setBusy(false) } }}>轮换 Token</Button>}
          </section>
        </aside>
      </div>
    </div>
  )
}

export { default as HistoryPage } from './History'

function SettingRow({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <div className="setting-row">
      <div>
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
      {children}
    </div>
  )
}

export function SettingsPage() {
  const { theme, setTheme, notify, runtime, font, fontSize, setFont, setFontSize } = useDesign()
  const [tab, setTab] = useState('general')
  const [restore, setRestore] = useState(() => localStorage.getItem('shellink.restore') !== 'false')
  const [size, setSize] = useState(fontSize)
  return (
    <div className="standard-page settings-page">
      <PageHeading
        eyebrow="PREFERENCES"
        title="设置"
        description="一个顺手的工具，应该懂你的习惯。"
      />
      <div className="settings-layout">
        <nav className="settings-nav" aria-label="设置分类">
          {[
            { id: 'general', label: '通用', icon: Settings2 },
            { id: 'appearance', label: '外观与终端', icon: Monitor },
            { id: 'webhook', label: 'Webhook 回调', icon: Webhook },
            { id: 'advanced', label: '高级', icon: Code2 },
          ].map((item) => (
            <button
              key={item.id}
              className={tab === item.id ? 'is-active' : ''}
              aria-pressed={tab === item.id}
              onClick={() => setTab(item.id)}
            >
              <item.icon size={15} />
              {item.label}
            </button>
          ))}
          <div className="settings-version">
            <span className="brand-mark small">
              <Terminal size={15} />
            </span>
            <strong>Shellink</strong>
            <span>{runtime?.status.version ?? '后台未连接'}</span>
          </div>
        </nav>
        <div className={cn('settings-content', tab === 'webhook' && 'settings-content-webhook')}>
          {tab === 'general' && (
            <>
              <div className="settings-section-title">
                <h2>工作区</h2>
                <p>打开即用，下次继续。</p>
              </div>
              <SettingRow title="启动时恢复工作区" description="重新打开上次的标签页和活动会话。">
                <Switch
                  checked={restore}
                  onCheckedChange={value => { setRestore(value); localStorage.setItem('shellink.restore', String(value)) }}
                  aria-label="启动时恢复工作区"
                />
              </SettingRow>
              <div className="settings-section-title">
                <h2>语言与数据</h2>
              </div>
              <SettingRow title="界面语言" description="选择 Shellink 显示的语言。">
                <Select
                  label="界面语言"
                  value="zh-CN"
                  onChange={() => notify('当前界面语言为简体中文。')}
                  options={[
                    { value: 'zh-CN', label: '简体中文' },
                    { value: 'en-US', label: 'English' },
                  ]}
                />
              </SettingRow>
              <SettingRow title="数据目录" description={runtime?.dataHome ?? '后台未连接'}>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!runtime}
                  onClick={() => runtime && void openDirectory(runtime.dataHome).catch(error => notify(String(error)))}
                >
                  <FolderOpen size={13} />
                  打开目录
                </Button>
              </SettingRow>
            </>
          )}
          {tab === 'appearance' && (
            <>
              <div className="settings-section-title">
                <h2>外观</h2>
                <p>安静的界面，让注意力回到终端。</p>
              </div>
              <div className="theme-previews">
                {(['dark', 'light'] as const).map((value) => (
                  <button
                    key={value}
                    aria-pressed={theme === value}
                    className={cn(
                      'theme-preview',
                      `theme-preview-${value}`,
                      theme === value && 'is-active',
                    )}
                    onClick={() => setTheme(value)}
                  >
                    <span className="theme-miniature">
                      <span />
                      <span>
                        <i />
                        <i />
                        <i />
                      </span>
                      <span />
                    </span>
                    <span>
                      {value === 'dark' ? <Moon size={13} /> : <Sun size={13} />}
                      {value === 'dark' ? '深色' : '浅色'}
                      {theme === value && <Check size={13} />}
                    </span>
                  </button>
                ))}
              </div>
              <SettingRow title="终端字体" description="为代码和命令输出选择等宽字体。">
                <Select
                  label="终端字体"
                  value={font}
                  onChange={setFont}
                  options={[
                    { value: 'JetBrains Mono', label: 'JetBrains Mono' },
                    { value: 'SF Mono', label: 'SF Mono' },
                    { value: 'Menlo', label: 'Menlo' },
                  ]}
                />
              </SettingRow>
              <SettingRow title="字体大小" description="调整终端文字的显示大小。">
                <Select
                  label="终端字体大小"
                  value={size}
                  onChange={value => { setSize(value); setFontSize(value) }}
                  options={['12', '13', '14', '16'].map((value) => ({
                    value,
                    label: `${value} px`,
                  }))}
                />
              </SettingRow>
              <div
                className="font-preview ui-selectable"
                style={{ fontFamily: `"${font}", monospace`, fontSize: `${size}px` }}
              >
                <span>deploy@prod-web-01</span> ~/app
                <br />❯ echo "Hello, Shellink."
                <br />
                <span>Hello, Shellink.</span>
              </div>
            </>
          )}
          {tab === 'webhook' && <WebhookMessages />}
          {tab === 'advanced' && (
            <>
              <div className="settings-section-title">
                <h2>自动化与后台</h2>
                <p>后台运行信息与本地集成入口。</p>
              </div>
              <SettingRow title="后台进程" description={`${runtime?.command ?? '未知可执行文件'} · PID ${runtime?.status.pid ?? '—'}`}>
                <Badge tone={runtime ? 'green' : 'neutral'}>
                  <span className="status-dot" />
                  {runtime ? '运行中' : '未连接'}
                </Badge>
              </SettingRow>
              <SettingRow title="HTTP 服务" description={runtime?.status.httpUrl ?? 'HTTP 服务未启用'}><Badge tone={runtime?.status.httpEnabled ? 'green' : 'neutral'}>{runtime?.status.httpEnabled ? '运行中' : '已关闭'}</Badge></SettingRow>
              <SettingRow title="活动会话" description="后台实时状态"><Badge>{runtime?.status.activeSessions ?? 0}</Badge></SettingRow>
              <SettingRow title="Webhook 回调消息" description="查看外部服务发送到 Shellink 的回调事件和消息原文。">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setTab('webhook')}
                >
                  管理
                </Button>
              </SettingRow>
              <SettingRow title="诊断信息" description="查看后台服务、版本和运行环境。">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => runtime ? notify(`版本 ${runtime.status.version} · PID ${runtime.status.pid} · ${runtime.status.activeSessions} 个活跃会话 · ${runtime.platform} · 日志 ${runtime.logPath}`) : notify('后台尚未连接。')}
                >
                  查看诊断
                </Button>
              </SettingRow>
              <div className="advanced-note">
                <ShieldCheck size={15} />
                <span>会话后台在应用窗口关闭后继续运行；系统 PATH 与登录启动尚未提供配置接口。</span>
              </div>
            </>
          )}
          <div className="settings-preview-note">
            <CircleHelp size={13} />
            偏好设置保存在此设备
          </div>
        </div>
      </div>
    </div>
  )
}
