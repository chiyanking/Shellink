import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ArrowRight,
  ArrowUpDown,
  ArrowUpRight,
  Box,
  Check,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  FileClock,
  Keyboard,
  Laptop,
  LayoutGrid,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  Sun,
  Terminal,
  PanelsTopLeft,
  Workflow,
  X,
} from 'lucide-react'
import { Design, type Modal } from './context'
import { connectionFrom, sessionFrom, emptyConnection, emptySession, type Connection, type Page, type Session } from './data'
import { rpc, reconnect, onCoreEvent, onDisconnect, errorText, preference, savePreference, type Profile, type SessionRecord, type RuntimeInfo, type MCPInfo, type CallbackRecord } from './backend'
import { Badge, Button, Dialog, Hint, Input, Menu, Select, TooltipProvider } from './components/ui'
import { cn } from './lib/utils'
import Workspace from './Workspace'
import { ConnectionsPage, HistoryPage, IntegrationsPage, SettingsPage } from './Pages'
import SessionsPage from './Sessions'

const navigation = [
  { id: 'workspace', label: '工作台', icon: LayoutGrid, shortcut: '1' },
  { id: 'sessions', label: '会话管理', icon: PanelsTopLeft, shortcut: '2' },
  { id: 'connections', label: '连接管理', icon: Server, shortcut: '3' },
  { id: 'integrations', label: 'AI 接入', icon: Workflow, shortcut: '4' },
  { id: 'history', label: '历史记录', icon: FileClock, shortcut: '5' },
] as const
const pageNames: Record<Page, string> = {
  workspace: '终端工作区',
  sessions: '会话管理',
  connections: '连接管理',
  integrations: 'AI 接入',
  history: '历史记录',
  settings: '设置',
}

export default function App() {
  const [page, setPage] = useState<Page>('workspace')
  const [sessions, setSessions] = useState<Session[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [runtime, setRuntime] = useState<RuntimeInfo | null>(null)
  const [mcp, setMcp] = useState<MCPInfo | null>(null)
  const [messages, setMessages] = useState<CallbackRecord[]>([])
  const [coreEpoch, setCoreEpoch] = useState(0)
  const [coreReady, setCoreReady] = useState(false)
  const readyRef = useRef(false)
  const [backendError, setBackendError] = useState('')
  const [loading, setLoading] = useState(true)
  const [selectedSessionId, setSelectedSessionId] = useState(() => preference('selectedSession', ''))
  const [inspectedSessionId, setInspectedSessionId] = useState('')
  const [historyScope, setHistoryScope] = useState<{ sessionId?: string; eventId?: string }>({})
  const [tabs, setTabs] = useState<string[]>(() => preference('restore', true) ? preference('tabs', []) : [])
  const [theme, setTheme] = useState<'dark' | 'light'>(() => preference('theme', 'dark'))
  const [font, setFont] = useState(() => preference('font', 'JetBrains Mono'))
  const [fontSize, setFontSize] = useState(() => preference('fontSize', '13'))
  const [sidebar, setSidebar] = useState(() => !window.matchMedia('(max-width: 580px)').matches)
  const [inspector, setInspector] = useState(true)
  const [split, setSplit] = useState(false)
  const [modal, setModal] = useState<Modal>(null)
  const [toast, setToast] = useState('')
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [collapsed, setCollapsed] = useState<string[]>([])
  const [commandQuery, setCommandQuery] = useState('')
  const [newSessionId, setNewSessionId] = useState('')
  const [newSessionName, setNewSessionName] = useState('')
  const [connectionType, setConnectionType] = useState('SSH')
  const [authType, setAuthType] = useState('password')
  const [connectionGroup, setConnectionGroup] = useState('Development')
  const [editingId, setEditingId] = useState('')
  const [saving, setSaving] = useState(false)
  const [importText, setImportText] = useState('')
  const selectedSession = sessions.find(item => item.id === selectedSessionId) ?? sessions.find(item => tabs.includes(item.id)) ?? emptySession
  const selected = connections.find(item => item.id === selectedSession.connectionId) ?? emptyConnection
  const manual = selectedSession.mode === 'MANUAL'
  const activeSessionCount = sessions.filter(item => item.status !== 'ended').length
  const refreshBusy = useRef(false)
  const notify = useCallback((text: string) => {
    setToast(text)
    if (toastTimer.current) clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setToast(''), 6000)
  }, [])
  const refresh = useCallback(async () => {
    if (refreshBusy.current) return
    refreshBusy.current = true
    try {
      const [info, profiles, records, mcpInfo, callbacks] = await Promise.all([
        rpc<RuntimeInfo>('desktop.info'), rpc<Profile[]>('profiles.list'), rpc<SessionRecord[]>('sessions.list'), rpc<MCPInfo>('mcp.info'), rpc<CallbackRecord[]>('webhooks.messages'),
      ])
      const mapped = profiles.map(profile => connectionFrom(profile, records))
      for (const record of records) {
        if (!mapped.some(item => item.id === record.profileId)) mapped.push({ ...emptyConnection, id: record.profileId, name: record.profileName || '已删除连接', host: record.target, archived: true })
      }
      setRuntime(info); setConnections(mapped); setSessions(records.map(sessionFrom)); setMcp(mcpInfo); setMessages(callbacks); if (readyRef.current) setBackendError('')
      setTabs(current => current.filter(id => records.some(record => record.id === id && record.active)))
      setNewSessionId(current => profiles.some(p => p.id === current) ? current : profiles[0]?.id ?? '')
    } catch (error) { setBackendError(errorText(error)) } finally { setLoading(false); refreshBusy.current = false }
  }, [])
  useEffect(() => {
    let alive = true; let subscription = ''; let timer: ReturnType<typeof setTimeout> | undefined
    const cleanups: (() => void)[] = []
    readyRef.current = false; setCoreReady(false)
    async function connect() {
      try {
        const eventsOff = await onCoreEvent(event => {
          if (alive && event.event !== 'session.data') { clearTimeout(timer); timer = setTimeout(() => { if (alive) void refresh() }, 100) }
        })
        if (!alive) { eventsOff(); return }; cleanups.push(eventsOff)
        const disconnectOff = await onDisconnect(reason => {
          if (alive) { readyRef.current = false; setCoreReady(false); setBackendError(reason) }
        })
        if (!alive) { disconnectOff(); return }; cleanups.push(disconnectOff)
        const result = await rpc<{ subscriptionId: string }>('events.subscribe', { replay: false })
        subscription = result.subscriptionId
        if (!alive) { await rpc('events.unsubscribe', { subscriptionId: subscription }); return }
        readyRef.current = true; setCoreReady(true)
        await refresh()
      } catch (error) { if (alive) { setBackendError(errorText(error)); setLoading(false) } }
    }
    void connect()
    const interval = setInterval(() => { if (alive && readyRef.current) void refresh() }, 4000)
    return () => { alive = false; readyRef.current = false; clearInterval(interval); clearTimeout(timer); cleanups.forEach(cleanup => cleanup()); if (subscription) void rpc('events.unsubscribe', { subscriptionId: subscription }).catch(() => {}) }
  }, [refresh, coreEpoch])
  async function retryBackend() {
    setLoading(true); setCoreReady(false); readyRef.current = false
    try { await reconnect(); setCoreEpoch(epoch => epoch + 1) } catch (error) { setBackendError(errorText(error)); setLoading(false) }
  }
  useEffect(() => { savePreference('tabs', tabs); savePreference('selectedSession', selectedSessionId) }, [tabs, selectedSessionId])
  useEffect(() => { savePreference('theme', theme); document.documentElement.classList.toggle('dark', theme === 'dark') }, [theme])
  useEffect(() => { savePreference('font', font); savePreference('fontSize', fontSize) }, [font, fontSize])
  const navigate = useCallback((next: Page) => {
    setPage(next); if (next === 'history') setHistoryScope({})
    if (window.matchMedia('(max-width: 580px)').matches) setSidebar(false)
  }, [])
  const openSession = useCallback((id: string) => {
    const session = sessions.find(item => item.id === id); if (!session) return
    if (session.status === 'ended') { setInspectedSessionId(id); navigate('sessions'); return }
    setSelectedSessionId(id); setTabs(current => current.includes(id) ? current : [...current, id]); navigate('workspace')
  }, [sessions, navigate])
  async function createSession(connectionId: string, name = '') {
    setSaving(true)
    try {
      const created = await rpc<{ id: string }>('sessions.create', { profileId: connectionId, name: name.trim(), source: 'desktop', mode: 'MANUAL' })
      await refresh(); setSelectedSessionId(created.id); setTabs(current => [...new Set([...current, created.id])]); navigate('workspace'); setNewSessionName(''); setModal(null)
    } catch (error) { notify(errorText(error)) } finally { setSaving(false) }
  }
  function openConnection(id: string) {
    const existing = sessions.find(item => item.connectionId === id && item.status !== 'ended')
    if (existing) openSession(existing.id); else void createSession(id)
  }
  function inspectSession(id: string) { setInspectedSessionId(id); navigate('sessions') }
  function inspectHistory(sessionId: string, eventId?: string) { navigate('history'); setHistoryScope({ sessionId, eventId }) }
  async function setSessionManual(id: string, value: boolean) {
    try { await rpc('sessions.mode', { id, mode: value ? 'MANUAL' : 'AUTO' }); await refresh() } catch (error) { notify(errorText(error)) }
  }
  function setManual(value: boolean) { if (selectedSession.id) void setSessionManual(selectedSession.id, value) }
  async function renameSession(id: string, name: string) { try { await rpc('sessions.rename', { id, name: name.trim() }); await refresh() } catch (error) { notify(errorText(error)); throw error } }
  async function endSessions(ids: string[]) {
    try { for (const id of ids) await rpc('sessions.close', { id }); await refresh(); setTabs(current => current.filter(id => !ids.includes(id))); notify(`已结束 ${ids.length} 个会话，终端历史保留。`) } catch (error) { notify(errorText(error)); await refresh(); throw error }
  }
  function closeTab(id: string) { const next = tabs.filter(tab => tab !== id); setTabs(next); if (selectedSessionId === id) setSelectedSessionId(next[0] ?? '') }
  function editConnection(id: string) {
    const profile = connections.find(item => item.id === id)?.profile
    if (!profile) return
    setEditingId(id); setConnectionType(profile.connectType === 'command' ? 'Local' : 'SSH'); setAuthType(profile.authType); setConnectionGroup(connections.find(item => item.id === id)!.group); setModal('connection')
  }
  async function deleteConnection(id: string) {
    try { await rpc('profiles.delete', { id }); await refresh(); notify('已删除连接配置，现有会话与记录保留。') } catch (error) { notify(errorText(error)) }
  }
  const editProfile = connections.find(item => item.id === editingId)?.profile
  useEffect(() => { if (modal !== 'connection') setEditingId('') }, [modal])
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); setCommandQuery(''); setModal(current => current === 'commands' ? null : 'commands') }
      if (event.shiftKey && event.key === 'Escape' && !modal && page === 'workspace') { event.preventDefault(); setManual(!manual) }
      if ((event.metaKey || event.ctrlKey) && /^[1-5]$/.test(event.key) && !modal) { event.preventDefault(); navigate(navigation[Number(event.key) - 1]!.id) }
    }
    window.addEventListener('keydown', handler); return () => window.removeEventListener('keydown', handler)
  }, [manual, modal, selectedSessionId, page, navigate])
  useEffect(() => () => { if (toastTimer.current) clearTimeout(toastTimer.current) }, [])

  const matchedConnections = connections.filter((connection) =>
    `${connection.name} ${connection.host}`.toLowerCase().includes(commandQuery.toLowerCase()),
  )
  const matchedSessions = sessions.filter((session) =>
    `${session.name} ${session.id}`.toLowerCase().includes(commandQuery.toLowerCase()),
  )
  return (
    <Design.Provider
      value={{
        coreEpoch, coreReady, connections, runtime, mcp, messages, refresh, editConnection, deleteConnection, font, fontSize, setFont, setFontSize,
        page,
        navigate,
        selected,
        sessions,
        selectedSession,
        openSession,
        inspectSession,
        inspectedSessionId,
        historyScope,
        inspectHistory,
        endSessions,
        renameSession,
        setSessionManual,
        openConnection,
        tabs,
        closeTab,
        manual,
        setManual,
        inspector,
        setInspector,
        split,
        setSplit,
        openModal: setModal,
        notify,
        theme,
        setTheme,
      }}
    >
      <TooltipProvider delayDuration={350}>
        <div className={cn('desktop-shell', !sidebar && 'sidebar-hidden')}>
          <header className="native-titlebar">
            <div className="native-window-controls" aria-hidden="true">
              <span />
              <span />
              <span />
            </div>
            <div className="native-preview">
              {loading ? '准备工作区' : backendError ? '连接中断' : 'DAEMON CONNECTED'} <span />
            </div>
          </header>
          <div className="desktop-body">
            <aside className="app-sidebar" aria-label="工作区导航">
              <Menu
                align="start"
                trigger={
                  <button className="workspace-switcher">
                    <span className="brand-mark">
                      <Terminal size={19} strokeWidth={2.4} />
                    </span>
                    <span>
                      <strong>Shellink</strong>
                      <small>个人工作区</small>
                    </span>
                    <ArrowUpDown size={13} />
                  </button>
                }
                items={[
                  { label: '个人工作区', icon: <Box size={14} />, checked: true },
                  {
                    label: '工作区设置',
                    icon: <Settings2 size={14} />,
                    onSelect: () => navigate('settings'),
                  },
                ]}
              />
              <nav className="main-navigation" aria-label="主导航">
                {navigation.map((item) => (
                  <button
                    key={item.id}
                    className={cn('main-nav-item', page === item.id && 'is-active')}
                    aria-current={page === item.id ? 'page' : undefined}
                    onClick={() => navigate(item.id)}
                  >
                    <item.icon size={16} />
                    <span>{item.label}</span>
                    {item.id === 'workspace' || item.id === 'sessions' ? (
                      <Badge>{item.id === 'workspace' ? tabs.length : activeSessionCount}</Badge>
                    ) : (
                      <kbd>⌘{item.shortcut}</kbd>
                    )}
                  </button>
                ))}
              </nav>
              <div className="sidebar-divider" />
              <div className="sidebar-section-heading">
                <span>我的连接</span>
                <Hint label="新建连接">
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="新建连接"
                    onClick={() => setModal('connection')}
                  >
                    <Plus size={14} />
                  </Button>
                </Hint>
              </div>
              <div className="sidebar-connections">
                {[...new Set(['Production', 'Development', 'Local', ...connections.filter(c => !c.archived).map(c => c.group)])].map((group) => (
                  <section className="connection-group" key={group}>
                    <button
                      className="connection-group-heading"
                      aria-expanded={!collapsed.includes(group)}
                      onClick={() =>
                        setCollapsed((current) =>
                          current.includes(group)
                            ? current.filter((value) => value !== group)
                            : [...current, group],
                        )
                      }
                    >
                      <ChevronDown
                        size={12}
                        className={collapsed.includes(group) ? 'is-collapsed' : ''}
                      />
                      <span>{group}</span>
                      <span>
                        {connections.filter((connection) => connection.group === group && !connection.archived).length}
                      </span>
                    </button>
                    {!collapsed.includes(group) &&
                      connections
                        .filter((connection) => connection.group === group && !connection.archived)
                        .map((connection) => (
                          <button
                            key={connection.id}
                            className={cn(
                              'sidebar-connection',
                              page === 'workspace' &&
                                tabs.length > 0 &&
                                selected.id === connection.id &&
                                'is-active',
                            )}
                            onClick={() => openConnection(connection.id)}
                            aria-pressed={
                              page === 'workspace' &&
                              tabs.length > 0 &&
                              selected.id === connection.id
                            }
                          >
                            <span
                              className={cn('sidebar-connection-icon', `color-${connection.color}`)}
                            >
                              {connection.type === 'Local' ? (
                                <Laptop size={14} />
                              ) : (
                                <Server size={14} />
                              )}
                            </span>
                            <span>{connection.name}</span>
                            {sessions.some(
                              (session) =>
                                session.connectionId === connection.id &&
                                session.status !== 'ended',
                            ) && <span className="status-dot" />}
                          </button>
                        ))}
                  </section>
                ))}
              </div>
              <button className="sidebar-add" onClick={() => setModal('connection')}>
                <Plus size={13} />
                添加新连接
              </button>
              <div className="sidebar-bottom">
                <button
                  className="sidebar-quick-open"
                  onClick={() => {
                    setCommandQuery('')
                    setModal('commands')
                  }}
                >
                  <Search size={13} />
                  <span>快速访问</span>
                  <kbd>⌘ K</kbd>
                </button>
                <button
                  className={cn('main-nav-item', page === 'settings' && 'is-active')}
                  onClick={() => navigate('settings')}
                  aria-current={page === 'settings' ? 'page' : undefined}
                >
                  <Settings2 size={16} />
                  <span>设置</span>
                </button>
                <div className="sidebar-user">
                  <span className="user-avatar">Y</span>
                  <div>
                    <strong>本地工作区</strong>
                    <span>
                      <ShieldCheck size={10} />
                      仅在你的设备上
                    </span>
                  </div>
                  <Hint label="后台连接信息">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="后台连接信息"
                      onClick={() =>
                        notify(runtime ? `后台 PID ${runtime.status.pid} · ${runtime.dataHome}` : backendError || '正在连接后台…')
                      }
                    >
                      <CircleHelp size={15} />
                    </Button>
                  </Hint>
                </div>
              </div>
            </aside>
            <main className="app-main">
              <header className="main-topbar">
                <div className="breadcrumbs">
                  <Hint label={sidebar ? '收起侧栏' : '展开侧栏'}>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={sidebar ? '收起侧栏' : '展开侧栏'}
                      onClick={() => setSidebar(!sidebar)}
                    >
                      {sidebar ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
                    </Button>
                  </Hint>
                  <span className="breadcrumb-divider" />
                  <span>工作区</span>
                  <ChevronRight size={12} />
                  <strong>{pageNames[page]}</strong>
                </div>
                <div className="topbar-actions">
                  <button
                    className="command-search"
                    onClick={() => {
                      setCommandQuery('')
                      setModal('commands')
                    }}
                  >
                    <Search size={13} />
                    <span>搜索命令或连接…</span>
                    <kbd>⌘ K</kbd>
                  </button>
                  <Hint label={theme === 'dark' ? '切换浅色外观' : '切换深色外观'}>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={theme === 'dark' ? '切换浅色外观' : '切换深色外观'}
                      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                    >
                      {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
                    </Button>
                  </Hint>
                  <span className="topbar-preview">
                    <span className="status-dot" />
                    {backendError ? '连接中断' : loading ? '连接中' : '后台已连接'}
                  </span>
                </div>
              </header>
              <div className="main-scroll" data-ui-scroll-container>
                {(backendError || loading) && <div className="backend-banner" role="status"><div><strong>{loading ? '正在准备工作区…' : '无法连接后台'}</strong><p>{backendError || '查找或启动本地服务，已有会话会继续运行。'}</p></div><Button variant="outline" disabled={loading} onClick={() => void retryBackend()}>重新连接</Button></div>}
                {page === 'workspace' && <Workspace />}
                {page === 'sessions' && <SessionsPage />}
                {page === 'connections' && <ConnectionsPage />}
                {page === 'integrations' && <IntegrationsPage />}
                {page === 'history' && (
                  <HistoryPage
                    key={`${historyScope.sessionId ?? 'all'}:${historyScope.eventId ?? ''}`}
                  />
                )}
                {page === 'settings' && <SettingsPage />}
              </div>
            </main>
          </div>
          <footer className="app-statusbar">
            <div>
              <span className="status-dot" />
              <span>本地工作区</span>
              <span className="status-separator">|</span>
              <button className="statusbar-session-link" onClick={() => navigate('sessions')}>
                {activeSessionCount} 个活跃会话
              </button>
            </div>
            <div className="statusbar-center">
              <Keyboard size={11} />
              <span>⇧ Esc 接管终端</span>
            </div>
            <div>
              <span>{runtime ? `PID ${runtime.status.pid}` : '未连接'}</span>
              <span className="status-separator">|</span>
              <span>Shellink {runtime?.status.version ?? '0.2.4'}</span>
              <span className="statusbar-brand">
                <Terminal size={11} />
              </span>
            </div>
          </footer>
        </div>
        <Dialog
          open={modal === 'connection'}
          onOpenChange={(open) => !open && setModal(null)}
          title={editingId ? '编辑连接' : '新建连接'}
          description="连接配置与凭证由本机后台加密保存。"
        >
          <form
            className="connection-form" key={`${editingId}:${modal === 'connection'}`}
            onSubmit={async (event) => {
              event.preventDefault(); const fields = new FormData(event.currentTarget); setSaving(true)
              const body: Record<string, unknown> = { name: String(fields.get('name')), connectType: connectionType === 'SSH' ? 'ssh' : 'command' }
              if (connectionType === 'SSH') { Object.assign(body, { host: String(fields.get('host')), port: Number(fields.get('port') || 22), username: String(fields.get('username')), authType }); const secret = String(fields.get('secret') || ''); if (secret) body[authType === 'key' ? 'privateKey' : 'password'] = secret; const passphrase = String(fields.get('passphrase') || ''); if (passphrase) body.passphrase = passphrase }
              else body.command = String(fields.get('command'))
              try { const result = await rpc<Profile>(editingId ? 'profiles.update' : 'profiles.create', editingId ? { id: editingId, profile: body } : body); savePreference(`group.${result.id}`, connectionType === 'Local' ? 'Local' : connectionGroup); await refresh(); setModal(null); notify('连接配置已保存') } catch (error) { notify(errorText(error)) } finally { setSaving(false) }
            }}
          >
            <div className="form-type-tabs">
              <button
                type="button"
                className={connectionType === 'SSH' ? 'is-active' : ''}
                onClick={() => setConnectionType('SSH')}
              >
                <Server size={15} />
                SSH 连接
              </button>
              <button
                type="button"
                className={connectionType === 'Local' ? 'is-active' : ''}
                onClick={() => setConnectionType('Local')}
              >
                <Terminal size={15} />
                本地命令
              </button>
            </div>
            <label className="form-field">
              连接名称
              <Input name="name" placeholder="例如 prod-web-01" defaultValue={editProfile?.name} required />
            </label>
            {connectionType === 'SSH' ? (
              <>
                <div className="form-row">
                  <label className="form-field">
                    主机地址
                    <Input name="host" placeholder="192.168.1.100" defaultValue={editProfile?.host} required />
                  </label>
                  <label className="form-field form-port">
                    端口
                    <Input name="port" placeholder="22" defaultValue={editProfile?.port ?? 22} type="number" min="1" max="65535" />
                  </label>
                </div>
                <label className="form-field">
                  用户名
                  <Input name="username" placeholder="deploy" defaultValue={editProfile?.username} required />
                </label>
                <div className="form-row">
                  <label className="form-field">
                    认证方式
                    <Select
                      label="认证方式"
                      value={authType}
                      onChange={setAuthType}
                      options={[
                        { value: 'key', label: 'SSH 密钥' },
                        { value: 'password', label: '密码' },
                      ]}
                    />
                  </label>
                  <label className="form-field">
                    分组
                    <Select
                      label="连接分组"
                      value={connectionGroup}
                      onChange={setConnectionGroup}
                      options={['Production', 'Development', 'Local'].map((value) => ({
                        value,
                        label: value,
                      }))}
                    />
                  </label>
                </div>
                <label className="form-field">
                  {authType === 'key' ? '私钥内容（留空保留原密钥）' : '密码（留空保留原密码）'}
                  {authType === 'key' ? <textarea name="secret" rows={4} className="ui-input" placeholder="粘贴 PEM / OpenSSH 私钥内容" /> : <Input name="secret" autoComplete="new-password" placeholder="输入密码" type="password" />}
                  {authType === 'key' && <Input name="passphrase" autoComplete="new-password" type="password" placeholder="私钥口令（可选）" />}
                </label>
              </>
            ) : (
              <label className="form-field">
                启动命令
                <Input name="command" placeholder={runtime?.defaultShell} defaultValue={editProfile?.command ?? runtime?.defaultShell ?? '/bin/sh'} required />
                <small>在本机启动的 Shell 或自定义命令。</small>
              </label>
            )}
            <div className="form-info">
              <ShieldCheck size={13} />
              保存配置不会立即连接。SSH 密码、私钥及口令在后台加密存储。
            </div>
            <div className="dialog-footer">
              <Button variant="outline" onClick={() => setModal(null)}>
                取消
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus size={14} />
                {saving ? '保存中…' : '保存连接'}
              </Button>
            </div>
          </form>
        </Dialog>
        <Dialog
          open={modal === 'session'}
          onOpenChange={(open) => !open && setModal(null)}
          title="新建会话"
          description="基于连接配置创建独立的终端实例。同一连接可以有多个会话。"
        >
          <label className="form-field session-name-field">
            会话名称 <span className="subtle">可选</span>
            <Input
              placeholder="例如：部署日志、生产服务巡检…"
              value={newSessionName}
              onChange={(event) => setNewSessionName(event.target.value)}
            />
          </label>
          <div className="session-picker">
            {connections.filter(c => !c.archived).map((connection) => (
              <button
                key={connection.id}
                className={cn('session-picker-item', newSessionId === connection.id && 'is-active')}
                onClick={() => setNewSessionId(connection.id)}
              >
                <span className={cn('connection-icon', `color-${connection.color}`)}>
                  {connection.type === 'SSH' ? <Server size={16} /> : <Terminal size={16} />}
                </span>
                <span>
                  <strong>{connection.name}</strong>
                  <small>
                    {connection.user}@{connection.host}
                  </small>
                </span>
                {newSessionId === connection.id && <Check size={15} />}
              </button>
            ))}
          </div>
          <div className="dialog-footer">
            <span className="subtle">独立会话 · 创建后由你控制</span>
            <Button
              disabled={!newSessionId || saving || !!backendError} onClick={() => void createSession(newSessionId, newSessionName)}
            >
              <Terminal size={14} />
              创建并打开
              <ArrowRight size={13} />
            </Button>
          </div>
        </Dialog>
        <Dialog
          open={modal === 'import'}
          onOpenChange={(open) => !open && setModal(null)}
          title="导入已有连接"
          description="把常用主机带入 Shellink，无需从头配置。"
        >
          <p>导入 Shellink 连接 JSON 对象或数组，文件内容仅发送到本机后台。导入会新增连接。</p>
          <input type="file" accept=".json,application/json" onChange={async event => { const file = event.target.files?.[0]; if (file) setImportText(await file.text()) }} />
          <textarea className="ui-input import-json" rows={10} value={importText} onChange={event => setImportText(event.target.value)} aria-label="连接 JSON" placeholder='[{"name":"本地终端","connectType":"command","command":"/bin/sh"}]' />
          <Button disabled={saving || !importText.trim()} onClick={async () => { setSaving(true); try { const json = JSON.parse(importText); const profiles = Array.isArray(json) ? json : [json]; for (const profile of profiles) await rpc('profiles.create', profile); await refresh(); setImportText(''); setModal(null); notify(`已导入 ${profiles.length} 个连接`) } catch (error) { await refresh(); notify(`导入中止，已成功写入的连接会保留：${errorText(error)}`) } finally { setSaving(false) } }}>导入连接</Button>
          <div className="dialog-footer">
            <Button variant="outline" onClick={() => setModal(null)}>
              关闭
            </Button>
          </div>
        </Dialog>
        <Dialog
          open={modal === 'commands'}
          onOpenChange={(open) => !open && setModal(null)}
          title="快速访问"
          description="搜索会话、连接或跳转到页面。"
          className="command-dialog"
        >
          <label className="command-dialog-search">
            <Search size={18} />
            <Input
              autoFocus
              placeholder="输入会话、连接或页面…"
              value={commandQuery}
              onChange={(event) => setCommandQuery(event.target.value)}
              aria-label="搜索命令和连接"
            />
            <kbd>ESC</kbd>
          </label>
          <div className="command-dialog-results">
            {matchedSessions.length > 0 && <div className="command-group-label">会话</div>}
            {matchedSessions.map((session) => (
              <button
                key={session.id}
                onClick={() => {
                  openSession(session.id)
                  setModal(null)
                }}
              >
                <PanelsTopLeft size={15} />
                <span>{session.name}</span>
                <small>{session.status === 'ended' ? '已结束 · 查看详情' : session.id}</small>
                <ArrowUpRight size={13} />
              </button>
            ))}
            {matchedConnections.length > 0 && <div className="command-group-label">连接</div>}
            {matchedConnections.map((connection) => (
              <button
                key={connection.id}
                onClick={() => {
                  openConnection(connection.id)
                  setModal(null)
                }}
              >
                <Server size={15} />
                <span>{connection.name}</span>
                <small>{connection.host}</small>
                <ArrowUpRight size={13} />
              </button>
            ))}
            {navigation.filter((item) => item.label.includes(commandQuery)).length > 0 && (
              <div className="command-group-label">页面</div>
            )}
            {navigation
              .filter((item) => item.label.includes(commandQuery))
              .map((item) => (
                <button
                  key={item.id}
                  onClick={() => {
                    setPage(item.id)
                    setModal(null)
                  }}
                >
                  <item.icon size={15} />
                  <span>{item.label}</span>
                  <kbd>⌘{item.shortcut}</kbd>
                </button>
              ))}
            {matchedSessions.length === 0 &&
              matchedConnections.length === 0 &&
              !navigation.some((item) => item.label.includes(commandQuery)) && (
                <div className="command-empty">没有找到匹配的会话、连接或页面</div>
              )}
          </div>
          <div className="command-dialog-footer">
            <span>
              <ArrowRight size={12} />
              选择项目后打开
            </span>
            <span>Shellink 快速访问</span>
          </div>
        </Dialog>
        {toast && (
          <div className="app-toast" role="status" aria-live="polite">
            <Check size={15} />
            <span>{toast}</span>
            <button onClick={() => setToast('')} aria-label="关闭提示">
              <X size={13} />
            </button>
          </div>
        )}
      </TooltipProvider>
    </Design.Provider>
  )
}
