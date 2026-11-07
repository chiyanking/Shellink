import { useState } from 'react'
import { rpc, openDirectory } from './backend'
import {
  ArrowRight,
  ArrowUpRight,
  Braces,
  Check,
  CircleHelp,
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
  X,
  ShieldCheck,
  Sun,
  Terminal,
  Trash2,
  Workflow,
  Zap,
} from 'lucide-react'
import { useDesign } from './context'
import { Badge, Button, Input, Menu, Select, Switch } from './components/ui'
import { cn } from './lib/utils'
import { PageHeading } from './Workspace'
import { settingsNavigation } from './Navigation'
import { connectionDragProps } from './connection-drag'

export function ConnectionsPage() {
  const {
    connections, openConnection, openModal, notify, sessions, inspectHistory, editConnection, deleteConnection,
    connectionGroup, setConnectionGroup,
    draggedConnectionId, setDraggedConnectionId,
  } = useDesign()
  const [search, setSearch] = useState('')
  const [sortByName, setSortByName] = useState(false)
  const profiles = connections.filter(item => !item.archived)
  const shown = profiles.filter(item =>
    (connectionGroup === null || item.group === connectionGroup) &&
    `${item.name} ${item.host} ${item.group}`.toLowerCase().includes(search.toLowerCase()),
  ).sort((a, b) => sortByName ? a.name.localeCompare(b.name) : 0)
  const active = sessions.filter(session => session.status !== 'ended')
  const emptyGroup = connectionGroup !== null && !profiles.some(item => item.group === connectionGroup)
  return <div className="standard-page connections-page">
    <PageHeading eyebrow="YOUR INFRASTRUCTURE" title="连接管理" description={`${profiles.length} 个连接 · ${profiles.filter(item => item.connected).length} 个连接有活跃会话 · 随时连接，继续工作。`} />
    <div className="connection-stats">
      {[
        { value: profiles.length, label: '全部连接', icon: Server, tone: '' },
        { value: active.length, label: '活跃会话', icon: Terminal, tone: 'green' },
        { value: profiles.filter(item => item.type === 'SSH').length, label: 'SSH 主机', icon: Globe, tone: 'purple' },
        { value: profiles.filter(item => item.type === 'Local').length, label: '本地连接', icon: Monitor, tone: 'cyan' },
      ].map(stat => <div className={`connection-stat tone-${stat.tone}`} key={stat.label}><div><strong>{stat.value.toString().padStart(2, '0')}</strong><stat.icon size={17} /></div><span>{stat.label}</span></div>)}
    </div>
    <div className="connections-toolbar">
      <div className="connections-group-filter">
        {connectionGroup !== null && <button className="group-filter-chip" onClick={() => setConnectionGroup(null)}>{connectionGroup || '未分组'}<X size={11} /></button>}
      </div>
      <div className="toolbar-right">
        <label className="search-input"><Search size={14} /><Input placeholder="搜索名称、地址或分组…" value={search} onChange={event => setSearch(event.target.value)} aria-label="搜索连接" /></label>
        <Menu trigger={<Button variant="outline" size="icon" aria-label="连接排序"><ListFilter size={15} /></Button>} items={[
          { label: '默认排序', checked: !sortByName, onSelect: () => setSortByName(false) },
          { label: '按名称排序', checked: sortByName, onSelect: () => setSortByName(true) },
        ]} />
      </div>
    </div>
    <div className="connection-card-grid">
      {shown.map(connection => {
        const activeSessions = active.filter(session => session.connectionId === connection.id)
        const latestSession = sessions.filter(session => session.connectionId === connection.id).sort((a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0))[0]
        const address = connection.type === 'SSH' ? `${connection.user}@${connection.host}:${connection.port}` : connection.command ?? 'localhost'
        return <article key={connection.id} className={cn('server-card', activeSessions.length > 0 && 'is-live', draggedConnectionId === connection.id && 'is-dragging')} {...connectionDragProps(connection.id, setDraggedConnectionId)}>
          <div className="server-card-top">
            <span className={cn('server-glyph', `color-${connection.color}`)}>{connection.type === 'Local' ? <Terminal size={20} /> : <Server size={20} />}</span>
            <div className="server-card-name"><h2 title={connection.name}>{connection.name}</h2><p title={address}>{address}</p></div>
            <Menu trigger={<Button variant="ghost" size="icon" aria-label={`${connection.name} 的更多操作`}><MoreHorizontal size={16} /></Button>} items={[
              { label: '编辑连接', icon: <Settings2 size={14} />, onSelect: () => editConnection(connection.id) },
              { label: '复制目标地址', icon: <Copy size={14} />, onSelect: () => void navigator.clipboard.writeText(address).then(() => notify('已复制目标地址')).catch(() => notify('复制失败')) },
              { label: '查看历史记录', icon: <FileClock size={14} />, onSelect: () => latestSession ? inspectHistory(latestSession.id) : notify('此连接尚无会话') },
              { label: '删除连接配置', icon: <Trash2 size={14} />, danger: true, onSelect: () => { if (window.confirm(`删除连接“${connection.name}”？现有会话和历史会保留。`)) void deleteConnection(connection.id) } },
            ]} />
          </div>
          <div className="server-card-tags">{connection.group && <span className={cn('environment-tag', connection.group === 'Production' ? 'is-production' : connection.type === 'Local' ? 'is-local' : 'is-development')}>{connection.group}</span>}<Badge>{connection.type === 'SSH' ? 'SSH' : 'LOCAL'}</Badge>{connection.profile?.authType && connection.type === 'SSH' && <span className="server-auth"><ShieldCheck size={11} />{connection.profile.authType === 'key' ? '密钥' : connection.profile.authType.includes('mfa') || connection.profile.authType === 'keyboard-interactive' ? 'MFA / 2FA' : '密码'}</span>}</div>
          <div className="server-card-bottom"><span className={cn('server-session-state', !activeSessions.length && 'is-idle')}><span className="status-dot" />{activeSessions.length ? `${activeSessions.length} 个活跃会话` : '尚无活跃会话'}</span><Button size="sm" variant={activeSessions.length ? 'default' : 'outline'} onClick={() => openConnection(connection.id)}>{activeSessions.length ? '打开终端' : '连接'}<ArrowUpRight size={12} /></Button></div>
        </article>
      })}
      {!emptyGroup && (shown.length > 0 || profiles.length === 0) && <button className="new-connection-card" onClick={() => openModal('connection')}><span><Plus size={22} /></span><strong>添加新连接</strong><small>SSH 主机或本地终端</small></button>}
    </div>
    {emptyGroup && <div className="empty-state"><FolderOpen size={25} /><h3>此分组还没有连接</h3><p>将连接拖入此分组，或添加一个新连接。</p><Button variant="outline" onClick={() => openModal('connection')}><Plus size={13} />添加连接</Button></div>}
    {!shown.length && profiles.length > 0 && !emptyGroup && <div className="empty-state"><Search size={25} /><h3>没有匹配的连接</h3><p>试试其他名称、地址或分组。</p><Button variant="outline" onClick={() => { setSearch(''); setConnectionGroup(null) }}>清除筛选</Button></div>}
    <div className="table-footer"><span>共 {shown.length} 个连接</span><span><ShieldCheck size={13} />连接信息仅保存在本地</span></div>
    <section className="import-banner"><div className="import-banner-icon"><FolderOpen size={22} /></div><div><h3>把熟悉的连接带过来</h3><p>导入 Shellink 连接 JSON，继续使用已有的主机配置。</p></div><Button variant="outline" onClick={() => openModal('import')}>导入配置<ArrowRight size={14} /></Button></section>
  </div>
}

export function IntegrationsPage() {
  const { notify, mcp, refresh, client, setClient } = useDesign()
  const [transport, setTransport] = useState('http')
  const [httpEnabled, setHttpEnabled] = useState(mcp?.httpEnabled ?? false)
  const [busy, setBusy] = useState(false)
  const executable = mcp?.command ?? ''
  const stdioEnv = mcp?.stdioEnv ?? {}
  const config = client === 'Codex'
    ? transport === 'http'
      ? `[mcp_servers.shellink]\nurl = ${JSON.stringify(mcp?.url ?? '')}\nhttp_headers = { Authorization = ${JSON.stringify(`Bearer ${mcp?.token ?? ''}`)} }`
      : `[mcp_servers.shellink]\ncommand = ${JSON.stringify(executable)}\nargs = ["mcp"]\nenv = { ${Object.entries(stdioEnv).map(([key, value]) => `${key} = ${JSON.stringify(value)}`).join(', ')} }`
    : JSON.stringify({
        mcpServers: {
          shellink: transport === 'http'
            ? { ...(client === 'Claude Code' ? { type: 'http' } : {}), url: mcp?.url ?? '', headers: { Authorization: `Bearer ${mcp?.token ?? ''}` } }
            : { command: executable, args: ['mcp'], env: stdioEnv },
        },
      }, null, 2)
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
        eyebrow="MCP INTEGRATIONS"
        title="MCP 接入"
        description="让外部 AI 客户端安全访问当前 Desktop 的连接和会话。"
      >
        <Badge className="preview-badge">
          <span className="status-dot" />
          本地 MCP
        </Badge>
      </PageHeading>
      <div className="integration-method" aria-label="接入方式">
        <Workflow size={16} />
        <span>Desktop MCP</span>
        <Badge tone="blue">推荐</Badge>
      </div>
      <div className="integration-layout">
        <section className="integration-setup">
          <div className="panel-title">
            <span className="step-number">1</span>
            <div>
              <h2>选择你的 AI 客户端</h2>
              <p>复制配置，添加到客户端的 MCP 设置。</p>
            </div>
          </div>
          <div className="client-choices">
            {([
              { name: 'Claude Code', symbol: '✳', className: 'client-claude' },
              { name: 'Codex', symbol: '⌘', className: 'client-codex' },
              { name: 'Cursor', symbol: '◈', className: 'client-cursor' },
            ] as const).map((item) => (
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
                className={transport === 'http' ? 'is-active' : ''}
                aria-pressed={transport === 'http'}
                onClick={() => setTransport('http')}
              >
                HTTP
              </button>
              <button
                className={transport === 'stdio' ? 'is-active' : ''}
                aria-pressed={transport === 'stdio'}
                onClick={() => setTransport('stdio')}
              >
                stdio
              </button>
            </div>
          </div>
          <div className="config-code">
            <div className="config-code-heading">
              <span>
                <Braces size={13} />
                {client === 'Codex' ? 'config.toml' : 'mcp.json'}
                <Badge>本机配置</Badge>
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => copy(config)}
              >
                <Copy size={13} />
                复制
              </Button>
            </div>
            <pre className="ui-selectable">
              <code>{config}</code>
            </pre>
          </div>
          <div className="integration-note">
            <CircleHelp size={14} />
            <span>{transport === 'stdio'
              ? mcp?.stdioRunning ? 'stdio 已就绪，关闭 HTTP 后仍可使用；需要保持 Shellink 桌面运行。' : mcp?.stdioError ?? 'stdio 尚未就绪，请检查桌面后台状态。'
              : mcp?.running ? 'HTTP 已启动，使用当前地址和 Token 连接。' : mcp?.error ?? 'HTTP 尚未启动，请开启右侧 MCP HTTP 开关。'}</span>
          </div>
          <div className="integration-next-step">
            <span className="step-number">2</span>
            <div>
              <h3>保存配置，重新连接客户端</h3>
              <p>接入后，AI 可发现已有主机并复用当前 Desktop 的终端会话。</p>
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
            <p>界面和 MCP 共享同一份终端状态。</p>
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
                <span>按会话查看保存的终端输入与输出</span>
              </div>
            </div>
            <div className="capability-footnote">终端与 MCP 共享当前 Desktop 工作区</div>
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
            <p>{mcp?.httpEnabled ? mcp.running ? 'HTTP 已启动 · Token 认证' : mcp.error ?? 'HTTP 启动失败' : 'HTTP MCP 已关闭，stdio 仍可使用'}</p>
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
  const { theme, setTheme, notify, runtime, font, fontSize, setFont, setFontSize, settingsTab: tab, setSettingsTab: setTab } = useDesign()
  const [restore, setRestore] = useState(() => localStorage.getItem('shellink.restore') !== 'false')
  const [size, setSize] = useState(fontSize)
  return (
    <div className="standard-page settings-page">
      <PageHeading
        eyebrow="PREFERENCES"
        title="设置"
        description="一个顺手的工具，应该懂你的习惯。"
      />
      <nav className="settings-mobile-nav" aria-label="设置分类">{settingsNavigation.map(item => <button key={item.id} className={tab === item.id ? 'is-active' : ''} aria-pressed={tab === item.id} onClick={() => setTab(item.id)}><item.icon size={14} />{item.label}</button>)}</nav>
      <div className="settings-layout">
        <div className="settings-content">
          {tab === 'general' && (
            <>
              <div className="settings-section-title">
                <h2>工作区</h2>
                <p>打开即用，下次继续。</p>
              </div>
              <SettingRow title="启动时恢复工作区" description="恢复上次查看的会话，在左侧列表切换其他会话。">
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
                {(['light', 'dark'] as const).map((value) => (
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
                      {value === 'dark' ? '墨蓝 · 靛青' : '纯白 · 靛蓝'}
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
          {tab === 'advanced' && (
            <>
              <div className="settings-section-title">
                <h2>自动化与后台</h2>
                <p>后台运行信息与本地集成入口。</p>
              </div>
                <SettingRow title="Desktop 核心" description={`${runtime?.command ?? '未知可执行文件'} · PID ${runtime?.status.pid ?? '—'}`}>
                <Badge tone={runtime ? 'green' : 'neutral'}>
                  <span className="status-dot" />
                  {runtime ? '运行中' : '未连接'}
                </Badge>
              </SettingRow>
              <SettingRow title="HTTP 服务" description={runtime?.status.httpUrl ?? 'HTTP 服务未启用'}><Badge tone={runtime?.status.httpEnabled ? 'green' : 'neutral'}>{runtime?.status.httpEnabled ? '运行中' : '已关闭'}</Badge></SettingRow>
                <SettingRow title="活动会话" description="Desktop 实时状态"><Badge>{runtime?.status.activeSessions ?? 0}</Badge></SettingRow>
                <SettingRow title="诊断信息" description="查看 Desktop 核心、版本和运行环境。">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => runtime ? notify(`版本 ${runtime.status.version} · PID ${runtime.status.pid} · ${runtime.status.activeSessions} 个活跃会话 · ${runtime.platform} · 日志 ${runtime.logPath}`) : notify('Desktop 尚未连接。')}
                >
                  查看诊断
                </Button>
              </SettingRow>
              <div className="advanced-note">
                <ShieldCheck size={15} />
                <span>会话和 MCP 服务随 Desktop 窗口运行，关闭窗口后会一并停止。</span>
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
