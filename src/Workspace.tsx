import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import {
  ArrowRight,
  ArrowUpRight,
  Bot,
  Copy,
  Ellipsis,
  FolderOpen,
  Hand,
  History,
  Keyboard,
  PanelRightClose,
  PanelsTopLeft,
  Plus,
  Radio,
  Server,
  ShieldCheck,
  SquareSplitHorizontal,
  Terminal,
  UserRound,
  X,
} from 'lucide-react'
import { useDesign } from './context'
import { emptyConnection, sessionStatus, type Session } from './data'
import { onCoreEvent, rpc } from './backend'
import { Badge, Button, Hint, Menu } from './components/ui'
import { cn } from './lib/utils'
import { createTerminalInput } from './terminal-input'

export function PageHeading({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow?: string
  title: string
  description: string
  children?: React.ReactNode
}) {
  return (
    <div className="page-heading">
      <div>
        {eyebrow && <div className="eyebrow">{eyebrow}</div>}
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="page-heading-actions">{children}</div>
    </div>
  )
}

function TerminalContent({ session }: { session: Session }) {
  const { coreEpoch, font, fontSize, notify } = useDesign()
  const control = useRef({ manual: false, active: false })
  control.current = { manual: session.mode === 'MANUAL', active: session.status !== 'ended' }
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!host.current || !session.id) return
    let alive = true
    const terminal = new XTerm({ convertEol: true, cursorBlink: true, fontFamily: `"${font}", monospace`, fontSize: Number(fontSize), theme: { background: '#101316', foreground: '#d9e0df' } })
    const fit = new FitAddon(); terminal.loadAddon(fit); terminal.open(host.current)
    let replaySeq = 0
    const buffered: { seq: number; raw: string }[] = []
    let replaying = true
    let dispose: (() => void) | undefined
    let resizeFrame = 0
    let sentCols = 0, sentRows = 0
    const resizeTerminal = () => {
      if (!alive || !host.current?.clientWidth || !host.current.clientHeight) return
      const size = fit.proposeDimensions()
      if (!size) return
      const cols = Math.max(20, Math.min(500, size.cols))
      const rows = Math.max(5, Math.min(200, size.rows))
      if (terminal.cols !== cols || terminal.rows !== rows) terminal.resize(cols, rows)
      if (control.current.active && (cols !== sentCols || rows !== sentRows)) {
        sentCols = cols; sentRows = rows
        void rpc('sessions.resize', { id: session.id, cols, rows }).catch(() => {})
      }
    }
    const resizeObserver = new ResizeObserver(() => {
      cancelAnimationFrame(resizeFrame)
      resizeFrame = requestAnimationFrame(resizeTerminal)
    })
    resizeObserver.observe(host.current)
    // Parse replay at the visible dimensions, rather than xterm's default 80×24.
    resizeTerminal()
    const sendInput = createTerminalInput(
      text => rpc('sessions.input', { id: session.id, text, manual: true, appendNewline: false }),
      error => { if (alive) notify(String(error)) },
    )
    const input = terminal.onData(data => {
      if (control.current.manual && control.current.active) sendInput(data)
    })
    void (async () => {
      const off = await onCoreEvent(event => {
        if (!alive || event.event !== 'session.data' || event.data.sessionId !== session.id || event.data.direction !== 'output' || typeof event.data.raw !== 'string') return
        const seq = Number(event.data.seq ?? event.seq ?? 0)
        if (replaying) buffered.push({ seq, raw: event.data.raw })
        else if (seq > replaySeq) { terminal.write(event.data.raw); replaySeq = seq }
      })
      if (!alive) { off(); return }
      dispose = off
      const replay = await rpc<{ raw: string; cursor: number }>('sessions.replay', { id: session.id }).catch(error => { if (alive) notify(String(error)); return null })
      if (!alive) return
      if (replay) { terminal.write(replay.raw); replaySeq = replay.cursor }
      replaying = false
      buffered.sort((a, b) => a.seq - b.seq).forEach(item => { if (item.seq > replaySeq) { terminal.write(item.raw); replaySeq = item.seq } })
    })().catch(error => { if (alive) notify(String(error)) })
    return () => { alive = false; dispose?.(); resizeObserver.disconnect(); cancelAnimationFrame(resizeFrame); input.dispose(); terminal.dispose() }
  }, [session.id, font, fontSize, coreEpoch, notify])
  return <div className="terminal-output xterm-host" aria-label={`${session.name} 终端`}><div ref={host} className="terminal-viewport" /></div>
}

function TerminalPanel({ session, secondary = false }: { session: Session; secondary?: boolean }) {
  const {
    tabs,
    sessions,
    selectedSession,
    connections,
    openSession,
    closeTab,
    setSessionManual,
    inspectSession,
    openModal,
    split,
    setSplit,
    notify,
  } = useDesign()
  const connection = connections.find((item) => item.id === session.connectionId) ?? emptyConnection
  const manual = session.mode === 'MANUAL'
  const setManual = (value: boolean) => setSessionManual(session.id, value)
  const [bottomTab, setBottomTab] = useState<'terminal' | 'files'>('terminal')
  return (
    <section
      className={cn('terminal-panel', secondary && 'terminal-panel-secondary')}
      aria-label={secondary ? '第二终端面板' : '终端面板'}
    >
      <div className="session-tabs">
        {secondary ? (
          <div className="session-tab is-active">
            <Terminal size={13} />
            <span>{session.name}</span>
          </div>
        ) : (
          tabs.map((id) => {
            const item = sessions.find((value) => value.id === id)
            if (!item) return null
            return (
              <div key={id} className={cn('session-tab', selectedSession.id === id && 'is-active')}>
                <button
                  className="session-tab-main"
                  onClick={() => openSession(id)}
                  aria-pressed={selectedSession.id === id}
                  title={`${item.name} · ${connections.find((connection) => connection.id === item.connectionId)?.name ?? '已删除连接'}`}
                >
                  <span className="status-dot" />
                  <span>{item.name}</span>
                </button>
                <button
                  className="session-tab-close"
                  aria-label={`收起 ${item.name} 标签`}
                  title="仅收起标签，会话继续运行"
                  onClick={() => closeTab(id)}
                >
                  <X size={12} />
                </button>
              </div>
            )
          })
        )}
        {!secondary && (
          <Hint label="新建会话">
            <Button
              variant="ghost"
              size="icon"
              className="session-add"
              aria-label="新建会话"
              onClick={() => openModal('session')}
            >
              <Plus size={15} />
            </Button>
          </Hint>
        )}
        <div className="session-tabs-end">
          <Menu
            trigger={
              <Button variant="ghost" size="icon" aria-label="终端选项">
                <Ellipsis size={16} />
              </Button>
            }
            items={[
              {
                label: '复制会话 ID',
                icon: <Copy size={14} />,
                onSelect: () => void navigator.clipboard.writeText(session.id).then(() => notify('已复制会话 ID')).catch(() => notify(session.id)),
              },
              {
                label: split ? '关闭分屏' : '拆分终端',
                icon: <SquareSplitHorizontal size={14} />,
                onSelect: () => setSplit(!split),
              },
              {
                label: '管理此会话',
                icon: <History size={14} />,
                onSelect: () => inspectSession(session.id),
              },
            ]}
          />
        </div>
      </div>
      <div className="terminal-address">
        <div>
          <ShieldCheck size={13} />
          <span>
            {connection.user}@{connection.host}
          </span>
          {connection.type === 'SSH' && <span className="terminal-port">:{connection.port}</span>}
          <Badge>{connection.type === 'Local' ? 'ZSH' : 'SSH'}</Badge>
        </div>
        <span className="terminal-ready">
          <span className="status-dot" />
          {sessionStatus[session.status].label}
        </span>
      </div>
      <div hidden={bottomTab !== 'terminal'}><TerminalContent session={session} /></div>
      {bottomTab === 'files' && <div className="terminal-files empty-state"><FolderOpen size={22} /><h3>文件浏览不可用</h3><p>当前后台协议未提供目录浏览接口。</p></div>}
      <div className={cn('terminal-input-notice', manual && 'is-manual')}>
        {session.status === 'ended' ? <div><History size={14} /><span>会话已结束 · {session.result} · 终端输出已保留</span></div> : <>
        <div>
          {manual ? <Keyboard size={14} /> : <Radio size={14} />}
          <span>
            {manual ? '你正在控制终端 · AI 写入已暂停' : 'AI 正在使用此终端，你可以随时接管。'}
          </span>
        </div>
        <button onClick={() => setManual(!manual)}>
          {manual ? '交还 AI' : '手动接管'}
          <ArrowRight size={12} />
        </button>
        </>}
      </div>
      <div className="terminal-bottom">
        <div className="terminal-bottom-tabs">
          <button
            className={bottomTab === 'terminal' ? 'is-active' : ''}
            onClick={() => setBottomTab('terminal')}
          >
            <Terminal size={12} />
            终端
          </button>
          <button
            className={bottomTab === 'files' ? 'is-active' : ''}
            onClick={() => setBottomTab('files')}
          >
            <FolderOpen size={12} />
            文件
          </button>
        </div>
        <span>
          {connection.type === 'Local' ? '本地终端' : 'SSH'} <span className="status-separator">/</span> UTF-8
        </span>
        <span className="terminal-latency">
          <span className="status-dot" />
          {sessionStatus[session.status].label}
        </span>
      </div>
    </section>
  )
}

function SessionInspector() {
  const {
    manual,
    setManual,
    setInspector,
    inspectHistory,
    selected,
    selectedSession,
    inspectSession,
    notify,
  } = useDesign()
  const [recentChunks, setRecentChunks] = useState<{id:number;seq:number;direction:string;plain:string;createdAt:number}[]>([])
  useEffect(() => {
    if (!selectedSession.id) { setRecentChunks([]); return }
    let active = true
    void import('./backend').then(({rpc}) => rpc<{items:{id:number;seq:number;direction:string;plain:string;createdAt:number}[]}>('sessions.historyChunks', {id:selectedSession.id,since:0,limit:100})).then(page => { if (active) setRecentChunks(page.items.slice(-3).reverse()) }).catch(() => { if (active) setRecentChunks([]) })
    return () => { active = false }
  }, [selectedSession.id])
  return (
    <aside className="session-inspector" aria-label="会话控制与活动">
      <div className="inspector-header">
        <span>会话面板</span>
        <Hint label="收起会话面板">
          <Button
            variant="ghost"
            size="icon"
            aria-label="收起会话面板"
            onClick={() => setInspector(false)}
          >
            <PanelRightClose size={15} />
          </Button>
        </Hint>
      </div>
      <div className="inspector-control">
        <div className="section-label">
          控制权 <span className="subtle">{manual ? 'MANUAL' : 'AUTO'}</span>
        </div>
        <div className="control-segments" aria-label="控制权模式">
          <button
            aria-pressed={!manual}
            className={!manual ? 'is-active' : ''}
            onClick={() => setManual(false)}
          >
            <Bot size={14} />
            AI 自动
          </button>
          <button
            aria-pressed={manual}
            className={manual ? 'is-active' : ''}
            onClick={() => setManual(true)}
          >
            <Hand size={14} />
            手动控制
          </button>
        </div>
        <div className="controller-info">
          <div className={cn('controller-avatar', manual && 'is-human')}>
            {manual ? <UserRound size={19} /> : <Bot size={19} />}
          </div>
          <div>
            <strong>{manual ? '你正在控制终端' : selectedSession.client}</strong>
            <span>{manual ? 'AI 仍可观察终端输出' : '终端已就绪，等待下一步指令'}</span>
          </div>
          <span className="status-dot" />
        </div>
        <Button
          className="takeover-button"
          aria-label={manual ? '交还 AI 控制权' : '接管终端'}
          variant={manual ? 'default' : 'outline'}
          onClick={() => setManual(!manual)}
        >
          {manual ? <Bot size={14} /> : <Hand size={14} />}
          {manual ? '交还 AI 控制权' : '接管终端'}
          <span>⇧ Esc</span>
        </Button>
        <p className="control-hint">
          {manual ? '此会话由人工控制，后台拒绝自动输入。' : '切换为手动控制后，可在终端直接输入。'}
        </p>
      </div>
      <div className="inspector-activity">
        <div className="section-label">
          终端片段{' '}
          <span className="live-indicator">
            <span />
            实时
          </span>
        </div>
        <div className="activity-timeline">
          {recentChunks.map((activity) => (
              <div className="activity-item" key={activity.id}>
                <div className="activity-icon"><Terminal size={12} /></div>
                <div className="activity-content">
                  <div className="activity-meta">
                    <span>来源未知 · {activity.direction}</span>
                    <time>{new Date(activity.createdAt).toLocaleTimeString()}</time>
                  </div>
                  <code className="activity-title">{activity.plain.slice(0, 180) || '(空片段)'}</code>
                </div>
              </div>
            ))}
          {!recentChunks.length && <span className="subtle">暂无历史片段</span>}
        </div>
        <button className="text-link" onClick={() => inspectHistory(selectedSession.id)}>
          查看此会话记录
          <ArrowUpRight size={13} />
        </button>
      </div>
      <div className="inspector-details">
        <div className="section-label">连接信息</div>
        <dl>
          <div>
            <dt>会话 ID</dt>
            <dd className="inspector-session-id">
              <code>{selectedSession.id}</code>
              <Button
                variant="ghost"
                size="icon"
                aria-label="复制 Session ID"
                title="复制 Session ID"
                onClick={() => void navigator.clipboard.writeText(selectedSession.id).then(() => notify('已复制会话 ID')).catch(() => notify(selectedSession.id))}
              >
                <Copy size={11} />
              </Button>
            </dd>
          </div>
          <div>
            <dt>连接方式</dt>
            <dd>{selected.type}</dd>
          </div>
          <div>
            <dt>连接时间</dt>
            <dd>{selectedSession.created.replace('今天 ', '')}</dd>
          </div>
          <div>
            <dt>状态</dt>
            <dd className="text-green">{sessionStatus[selectedSession.status].label}</dd>
          </div>
        </dl>
        <button
          className="text-link inspector-manage"
          onClick={() => inspectSession(selectedSession.id)}
        >
          管理此会话
          <ArrowUpRight size={12} />
        </button>
      </div>
    </aside>
  )
}

export default function Workspace() {
  const {
    selectedSession,
    connections,
    sessions,
    tabs,
    inspector,
    split,
    openModal,
    openSession,
    navigate,
  } = useDesign()
  const otherSession = sessions.find(
    (item) => item.id !== selectedSession.id && item.status !== 'ended',
  )
  if (!tabs.length || !selectedSession.id)
    return (
      <div className="workspace-page">
        <PageHeading title="终端工作区" description="在这里打开会话，继续你的工作。">
          <Button onClick={() => openModal('session')}>
            <Plus size={15} />
            新建会话
          </Button>
        </PageHeading>
        <div className="workspace-empty empty-state">
          <PanelsTopLeft size={34} />
          <h3>标签已收起，会话仍在这里</h3>
          <p>前往会话管理，重新打开任意活跃会话。</p>
          <Button variant="outline" onClick={() => navigate('sessions')}>
            进入会话管理
            <ArrowRight size={13} />
          </Button>
          <span className="subtle">后台暂无活跃会话</span>
        </div>
      </div>
    )
  return (
    <div className="workspace-page workspace-terminal-page">
      <div className={cn('workspace-grid', !inspector && 'without-inspector')}>
        <div className="workspace-main">
          <div className={cn('terminal-panes', split && 'is-split')}>
            <TerminalPanel session={selectedSession} />
            {split && otherSession && <TerminalPanel session={otherSession} secondary />}
          </div>
          <section className="recent-sessions">
            <div className="recent-heading">
              <h2>最近会话</h2>
              <button className="text-link subtle" onClick={() => navigate('sessions')}>
                全部会话
                <ArrowRight size={12} />
              </button>
            </div>
            <div className="recent-session-grid">
              {sessions
                .filter((item) => item.status !== 'ended')
                .slice(0, 4)
                .map((session) => {
                  const connection = connections.find((item) => item.id === session.connectionId) ?? emptyConnection
                  return (
                    <button
                      key={session.id}
                      className="recent-session"
                      onClick={() => openSession(session.id)}
                    >
                      <span className={cn('connection-icon', `color-${connection.color}`)}>
                        <Server size={16} />
                      </span>
                      <div>
                        <strong>{session.name}</strong>
                        <span>
                          {connection.name} · {session.client}
                        </span>
                      </div>
                      <span className="recent-state">
                        <span className="status-dot" />
                        {sessionStatus[session.status].label}
                      </span>
                      <ArrowUpRight size={14} className="subtle" />
                    </button>
                  )
                })}
            </div>
          </section>
          <div className="workspace-footnote">
            <ShieldCheck size={13} />
            <span>收起标签仅关闭视图；结束会话请前往会话管理。</span>
          </div>
        </div>
        {inspector && <SessionInspector />}
      </div>
    </div>
  )
}
