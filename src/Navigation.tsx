import { useState, type DragEvent } from 'react'
import {
  ArrowUpRight, Bot, ChevronDown, Code2, Copy, FileClock, Globe, Keyboard,
  FolderPlus, Monitor, Plus, Search, Server, Settings2, ShieldCheck,
  Terminal, Trash2, Workflow,
} from 'lucide-react'
import { useDesign, type SettingsTab } from './context'
import { shortTime } from './data'
import { Badge, Hint, Input } from './components/ui'
import { cn } from './lib/utils'
import { connectionDragProps } from './connection-drag'

export const navigation = [
  { id: 'workspace', label: '工作台', title: 'Workspace', icon: Terminal, shortcut: '1' },
  { id: 'connections', label: '连接管理', title: 'Connections', icon: Server, shortcut: '2' },
  { id: 'integrations', label: 'AI 接入', title: 'Integrations', icon: Workflow, shortcut: '3' },
  { id: 'history', label: '历史记录', title: 'History', icon: FileClock, shortcut: '4' },
] as const

export const settingsNavigation: { id: SettingsTab; label: string; icon: typeof Settings2; description: string }[] = [
  { id: 'general', label: '通用', icon: Settings2, description: '工作区、语言与数据' },
  { id: 'appearance', label: '外观与终端', icon: Monitor, description: '主题、字体与显示' },
  { id: 'advanced', label: '高级', icon: Code2, description: '后台与运行信息' },
]

export function NavigationRail() {
  const { page, navigate, openModal, runtime } = useDesign()
  return <nav className="navigation-rail" aria-label="主导航">
    <Hint label="Shellink · 个人工作区">
      <button className="rail-logo" aria-label="Shellink 工作台" onClick={() => navigate('workspace')}><Terminal size={20} strokeWidth={2.4} /></button>
    </Hint>
    {navigation.map(item => <Hint key={item.id} label={`${item.label} · ⌘${item.shortcut}`}>
      <button className={cn('rail-button', page === item.id && 'is-active')} aria-label={item.label} aria-current={page === item.id ? 'page' : undefined} onClick={() => navigate(item.id)}><item.icon size={19} /></button>
    </Hint>)}
    <Hint label="设置"><button className={cn('rail-button', page === 'settings' && 'is-active')} aria-label="设置" aria-current={page === 'settings' ? 'page' : undefined} onClick={() => navigate('settings')}><Settings2 size={19} /></button></Hint>
    <div className="rail-spacer" />
    <Hint label="快速访问 · ⌘K"><button className="rail-button" aria-label="快速访问" onClick={() => openModal('commands')}><Search size={18} /></button></Hint>
    <Hint label={runtime ? '本地工作区 · 已连接' : '本地工作区'}><button className="rail-avatar" aria-label="本地工作区设置" onClick={() => navigate('settings')}>S<span className={cn('avatar-presence', !runtime && 'is-offline')} /></button></Hint>
  </nav>
}

export function ContextPanel() {
  const {
    page, sessions, selectedSession, openSession, openModal, connections, openConnection, editConnection,
    connectionGroup, setConnectionGroup,
    connectionGroups, addConnectionGroup, deleteConnectionGroup, moveConnectionToGroup, draggedConnectionId, setDraggedConnectionId,
    mcp, client, setClient, settingsTab, setSettingsTab, historyScope, inspectHistory, navigate, runtime, notify,
  } = useDesign()
  const [search, setSearch] = useState('')
  const [collapsed, setCollapsed] = useState<string[]>([])
  const [dropGroup, setDropGroup] = useState<string | null>(null)
  const [newGroupName, setNewGroupName] = useState('')
  const [creatingGroup, setCreatingGroup] = useState(false)
  const [groupError, setGroupError] = useState('')
  const active = sessions.filter(session => session.status !== 'ended')
  const ended = sessions.filter(session => session.status === 'ended')
  const profiles = connections.filter(connection => !connection.archived)
  const matches = (text: string) => text.toLowerCase().includes(search.toLowerCase())
  function dropProps(group: string) {
    return {
      onDragOver: (event: DragEvent<HTMLElement>) => {
        if (!draggedConnectionId) return
        event.preventDefault()
        event.dataTransfer.dropEffect = 'move'
        setDropGroup(group)
      },
      onDragLeave: (event: DragEvent<HTMLElement>) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropGroup(null)
      },
      onDrop: (event: DragEvent<HTMLElement>) => {
        if (!draggedConnectionId) return
        event.preventDefault()
        moveConnectionToGroup(draggedConnectionId, group)
        setDraggedConnectionId(null)
        setDropGroup(null)
      },
    }
  }
  function connectionList(group: string) {
    return profiles.filter(connection => connection.group === group).map(connection => <button
      className={cn('context-item context-connection', draggedConnectionId === connection.id && 'is-dragging')}
      key={connection.id} onClick={() => editConnection(connection.id)} title={`${connection.name} · ${connection.host} · 点击编辑，可拖动到分组`}
      {...connectionDragProps(connection.id, setDraggedConnectionId)}
    ><span className={cn('context-dot', connection.connected ? 'is-live' : 'is-ended')} /><span className="context-item-text"><strong>{connection.name}</strong></span><span className="context-count">{connection.type}</span></button>)
  }
  function sessionList(items: typeof sessions) {
    return items.filter(session => matches(`${session.name} ${session.id}`)).map(session => {
      const selected = page === 'history' ? historyScope.sessionId === session.id : selectedSession.id === session.id
      return <div key={session.id} className={cn('context-item context-session', selected && 'is-active')}>
        <button
          className="context-session-select"
          aria-pressed={selected}
          onClick={() => page === 'history' ? inspectHistory(session.id) : openSession(session.id)}
          title={`${session.name} · ${session.id}`}
        >
          <span className={cn('context-dot', session.status === 'ended' ? 'is-ended' : session.status === 'attention' ? 'is-attention' : 'is-live')} />
          <span className="context-item-text"><strong>{session.name}</strong><small>{connections.find(connection => connection.id === session.connectionId)?.name ?? '已删除连接'} · {session.mode} · {session.createdAt ? shortTime(session.createdAt) : session.created}</small></span>
          {session.status !== 'ended' && session.mode === 'AUTO' && <Bot size={12} className="context-bot" />}
        </button>
        <Hint label={`复制会话 ID · ${session.id}`}>
          <button
            className="context-session-copy"
            aria-label={`复制 ${session.name} 的会话 ID`}
            onClick={() => void navigator.clipboard.writeText(session.id).then(() => notify('已复制会话 ID')).catch(() => notify('复制失败，请重试'))}
          ><Copy size={12} /></button>
        </Hint>
      </div>
    })
  }
  return <aside className="context-panel" aria-label={`${page === 'workspace' ? '会话' : page === 'connections' ? '连接' : page === 'integrations' ? 'AI 接入' : page === 'history' ? '历史' : '设置'}侧面板`}>
    <div className="context-heading"><span>{page === 'workspace' ? '会话' : page === 'connections' ? '连接' : page === 'integrations' ? 'AI 客户端' : page === 'history' ? '终端历史' : '偏好设置'}</span>
    </div>
    <div className="context-scroll">
      {(page === 'workspace' || page === 'history') && <>
        <label className="context-search"><Search size={13} /><input aria-label="搜索会话" placeholder="搜索会话…" value={search} onChange={event => setSearch(event.target.value)} /><kbd>⌕</kbd></label>
        <section className="context-group"><div className="context-label">运行中 <span>{active.length}</span></div>{sessionList(active)}{!active.length && <p className="context-empty">暂无运行中的会话</p>}</section>
        <section className="context-group"><div className="context-label">已结束 <span>{ended.length}</span></div>{sessionList(ended)}{!ended.length && <p className="context-empty">结束的会话会保留在这里</p>}</section>
        {!!search && !sessions.some(session => matches(`${session.name} ${session.id}`)) && <p className="context-empty">没有匹配的会话</p>}
        {page === 'workspace' && <div className="context-group"><div className="context-label">快速连接 <span>{profiles.length}</span></div>{profiles.slice(0, 5).map(connection => <button className="context-item" key={connection.id} onClick={() => openConnection(connection.id)}><Server size={13} /><span className="context-item-text"><strong>{connection.name}</strong></span><ArrowUpRight size={12} /></button>)}{!profiles.length && <button className="context-text-button" onClick={() => openModal('connection')}><Plus size={12} />添加你的第一个连接</button>}</div>}
      </>}
      {page === 'connections' && <>
        <section className={cn('context-group context-ungrouped', draggedConnectionId && 'is-drop-zone', draggedConnectionId && dropGroup === '' && 'is-drop-target')} aria-label="分组外的连接" {...dropProps('')}>
          {connectionList('')}
          {draggedConnectionId && <p className="context-drop-hint">拖到这里移出分组</p>}
        </section>
        {connectionGroups.map(group => <section className={cn('context-group context-connection-group', draggedConnectionId && 'is-drop-zone', draggedConnectionId && dropGroup === group && 'is-drop-target')} key={group} aria-label={`连接分组 ${group}`} {...dropProps(group)}>
          <div className={cn('context-group-heading', connectionGroup === group && 'is-active')}>
            <button className="context-group-toggle" onClick={() => setCollapsed(current => current.includes(group) ? current.filter(value => value !== group) : [...current, group])} aria-label={`${collapsed.includes(group) ? '展开' : '收起'}分组 ${group}`} aria-expanded={!collapsed.includes(group)}><ChevronDown size={12} className={collapsed.includes(group) ? 'is-collapsed' : ''} /></button>
            <button className="context-group-select" aria-pressed={connectionGroup === group} onClick={() => { setConnectionGroup(group); setCollapsed(current => current.filter(value => value !== group)) }}><strong title={group}>{group}</strong><span>{profiles.filter(connection => connection.group === group).length}</span></button>
            <Hint label={profiles.some(connection => connection.group === group) ? '请先移出分组内的连接，再删除分组' : '删除分组'}><button className="context-group-delete" aria-label={`删除分组 ${group}`} onClick={() => {
              if (deleteConnectionGroup(group)) {
                setCollapsed(current => current.filter(value => value !== group))
              }
            }}><Trash2 size={12} /></button></Hint>
          </div>
          {!collapsed.includes(group) && connectionList(group)}
          {draggedConnectionId && !profiles.some(connection => connection.group === group) && <p className="context-drop-hint">拖到这里加入分组</p>}
        </section>)}
        {creatingGroup ? <form className="context-group-create" aria-label="新建分组" onSubmit={event => {
          event.preventDefault()
          const error = addConnectionGroup(newGroupName)
          setGroupError(error ?? '')
          if (!error) { setNewGroupName(''); setCreatingGroup(false) }
        }}>
          <FolderPlus size={14} />
          <Input autoFocus aria-label="分组名称" placeholder="分组名称，回车创建" value={newGroupName} maxLength={50} onChange={event => { setNewGroupName(event.target.value); setGroupError('') }} onKeyDown={event => {
            if (event.key === 'Escape') { event.preventDefault(); setCreatingGroup(false); setNewGroupName(''); setGroupError('') }
          }} onBlur={() => { setCreatingGroup(false); setNewGroupName(''); setGroupError('') }} />
        </form> : <button className="context-add-group" onClick={() => setCreatingGroup(true)}><Plus size={13} />新建分组</button>}
        {groupError && <p className="context-group-error" role="alert">{groupError}</p>}
      </>}
      {page === 'integrations' && <>
        {(['Claude Code', 'Cursor', 'Codex'] as const).map(name => <button className={cn('context-item context-client', client === name && 'is-active')} aria-pressed={client === name} key={name} onClick={() => setClient(name)}><span className={cn('context-client-glyph', `glyph-${name.split(' ')[0].toLowerCase()}`)}>{name === 'Claude Code' ? '✳' : name === 'Cursor' ? '◈' : '⌘'}</span><span className="context-item-text">{name}</span>{client === name && <Badge tone="blue">配置中</Badge>}</button>)}
        <div className="context-label">传输服务</div>
        <div className="context-service"><Globe size={14} /><span>HTTP</span><Badge tone={mcp?.running ? 'green' : 'neutral'}>{mcp?.running ? '运行中' : '未启动'}</Badge></div>
        <div className="context-service"><Terminal size={14} /><span>stdio</span><Badge tone={mcp?.stdioRunning ? 'green' : 'neutral'}>{mcp?.stdioRunning ? '已就绪' : '未就绪'}</Badge></div>
      </>}
      {page === 'settings' && settingsNavigation.map(item => <button key={item.id} className={cn('context-item context-setting', settingsTab === item.id && 'is-active')} aria-pressed={settingsTab === item.id} onClick={() => { setSettingsTab(item.id); navigate('settings') }}><item.icon size={15} /><span className="context-item-text"><strong>{item.label}</strong><small>{item.description}</small></span></button>)}
      <div className="context-spacer" />
      <section className="context-card">
        <h3>{page === 'workspace' ? <><Bot size={14} />共享终端，随时接管</> : page === 'integrations' ? <><Workflow size={14} />连接你的 AI 工具</> : <><ShieldCheck size={14} />你的设备，你的工作区</>}</h3>
        <p>{page === 'workspace' ? '通过 MCP 与 AI 共用终端。切换到手动控制，即可暂停 AI 写入。' : page === 'integrations' ? '选择客户端，复制当前工作区的 MCP 配置，即可开始终端协作。' : '连接配置、终端历史与偏好设置保存在本机。'}</p>
        {page === 'workspace' && <button onClick={() => navigate('integrations')}>配置 AI 接入<ArrowUpRight size={11} /></button>}
      </section>
      {page === 'settings' && <div className="context-version"><Terminal size={12} />Shellink<span>v{runtime?.status.version ?? '0.2.4'}</span></div>}
      <button className="context-shortcut" onClick={() => openModal('commands')}><Keyboard size={12} /><span>快速访问</span><kbd>⌘ K</kbd></button>
    </div>
  </aside>
}
