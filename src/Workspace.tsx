import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import {
  ArrowRight,
  Bot,
  Copy,
  Ellipsis,
  FolderOpen,
  Hand,
  History,
  Keyboard,
  PanelsTopLeft,
  Plus,
  Radio,
  ShieldCheck,
  SquareSplitHorizontal,
  Terminal,
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
        <div className="terminal-connection">
          <ShieldCheck size={13} />
          <span>
            {connection.user}@{connection.host}
          </span>
          {connection.type === 'SSH' && <span className="terminal-port">:{connection.port}</span>}
          <Badge>{connection.type === 'Local' ? 'ZSH' : 'SSH'}</Badge>
        </div>
        <div className="terminal-session-meta">
          <div className="terminal-control" role="group" aria-label="控制权模式">
            <span className="terminal-meta-label">控制权</span>
            <button
              className={cn('terminal-control-option', !manual && 'is-active')}
              aria-pressed={!manual}
              disabled={session.status === 'ended'}
              onClick={() => setManual(false)}
            >
              <Bot size={12} />
              AI 自动
            </button>
            <button
              className={cn('terminal-control-option', manual && 'is-active')}
              aria-pressed={manual}
              disabled={session.status === 'ended'}
              onClick={() => setManual(true)}
            >
              <Hand size={12} />
              手动控制
            </button>
          </div>
          <div className="terminal-session-id" title={session.id}>
            <span className="terminal-meta-label">Session ID</span>
            <code>{session.id}</code>
            <Button
              variant="ghost"
              size="icon"
              aria-label="复制 Session ID"
              title="复制 Session ID"
              onClick={() => void navigator.clipboard.writeText(session.id).then(() => notify('已复制会话 ID')).catch(() => notify(session.id))}
            >
              <Copy size={12} />
            </Button>
          </div>
          <span className="terminal-ready">
            <span className="status-dot" />
            {sessionStatus[session.status].label}
          </span>
        </div>
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

export default function Workspace() {
  const {
    selectedSession,
    sessions,
    tabs,
    split,
    openModal,
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
      <div className="workspace-grid">
        <div className="workspace-main">
          <div className={cn('terminal-panes', split && 'is-split')}>
            <TerminalPanel session={selectedSession} />
            {split && otherSession && <TerminalPanel session={otherSession} secondary />}
          </div>
        </div>
      </div>
    </div>
  )
}
