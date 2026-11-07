import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import '@xterm/xterm/css/xterm.css'
import {
  ArrowRight,
  Bot,
  Ellipsis,
  Hand,
  History,
  Keyboard,
  PanelsTopLeft,
  Plus,
  Radio,
  Server,
  Workflow,
  ShieldCheck,
  SquareSplitHorizontal,
  Terminal,
} from 'lucide-react'
import { useDesign } from './context'
import { createCommandHighlighter, terminalTheme } from './terminal-appearance'
import { emptyConnection, sessionStatus, type Session } from './data'
import { onCoreEvent, rpc } from './backend'
import { Badge, Button, Menu } from './components/ui'
import { cn } from './lib/utils'
import { createTerminalInput } from './terminal-input'

const TERMINAL_SCROLLBAR_WIDTH = 6

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
  const { coreEpoch, font, fontSize, notify, theme: colorScheme } = useDesign()
  const fontDescriptor = `${fontSize}px "${font}"`
  const [readyFont, setReadyFont] = useState('')
  const control = useRef({ manual: false, active: false })
  control.current = { manual: session.mode === 'MANUAL', active: session.status !== 'ended' }
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let alive = true
    // Font swapping after open leaves xterm's cached glyph widths out of date.
    void Promise.all([
      document.fonts.load(fontDescriptor),
      document.fonts.load(`bold ${fontDescriptor}`),
    ]).catch(() => {}).then(() => { if (alive) setReadyFont(fontDescriptor) })
    return () => { alive = false }
  }, [fontDescriptor])
  useEffect(() => {
    if (!host.current || !session.id || readyFont !== fontDescriptor) return
    let alive = true
    const terminal = new XTerm({
      convertEol: true,
      cursorBlink: true,
      cursorStyle: 'bar',
      cursorWidth: 2,
      fontFamily: `"${font}", monospace`,
      fontSize: Number(fontSize),
      lineHeight: 1.35,
      // xterm 6 uses this width for both its scrollbar and FitAddon's column budget.
      overviewRuler: { width: TERMINAL_SCROLLBAR_WIDTH },
      theme: terminalTheme(colorScheme),
    })
    const highlight = createCommandHighlighter(colorScheme)
    const write = (raw: string) => terminal.write(highlight(raw))
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
    let webgl: WebglAddon | undefined
    try {
      webgl = new WebglAddon()
      webgl.onContextLoss(() => {
        // Disposing the addon restores xterm's default renderer.
        webgl?.dispose()
        resizeTerminal()
      })
      terminal.loadAddon(webgl)
    } catch {
      // Keep the terminal usable when WebGL2 is unavailable.
      webgl?.dispose()
    }
    const resizeObserver = new ResizeObserver(() => {
      cancelAnimationFrame(resizeFrame)
      resizeFrame = requestAnimationFrame(resizeTerminal)
    })
    resizeObserver.observe(host.current)
    // Parse replay at the visible dimensions, rather than xterm's default 80×24.
    resizeTerminal()
    const sendInput = createTerminalInput(
      text => rpc('sessions.input', { id: session.id, text, manual: true }),
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
        else if (seq > replaySeq) { write(event.data.raw); replaySeq = seq }
      })
      if (!alive) { off(); return }
      dispose = off
      const replay = await rpc<{ raw: string; cursor: number }>('sessions.replay', { id: session.id }).catch(error => { if (alive) notify(String(error)); return null })
      if (!alive) return
      if (replay) { write(replay.raw); replaySeq = replay.cursor }
      replaying = false
      buffered.sort((a, b) => a.seq - b.seq).forEach(item => { if (item.seq > replaySeq) { write(item.raw); replaySeq = item.seq } })
    })().catch(error => { if (alive) notify(String(error)) })
    return () => { alive = false; dispose?.(); resizeObserver.disconnect(); cancelAnimationFrame(resizeFrame); input.dispose(); terminal.dispose() }
  }, [session.id, font, fontSize, fontDescriptor, readyFont, coreEpoch, notify, colorScheme])
  return <div className="terminal-output xterm-host" aria-label={`${session.name} 终端`}><div ref={host} className="terminal-viewport" /></div>
}

function TerminalPanel({ session, secondary = false }: { session: Session; secondary?: boolean }) {
  const {
    sessions,
    connections,
    setSessionManual,
    inspectHistory,
    split,
    setSplit,
    notify,
  } = useDesign()
  const connection = connections.find((item) => item.id === session.connectionId) ?? emptyConnection
  const manual = session.mode === 'MANUAL'
  const setManual = (value: boolean) => setSessionManual(session.id, value)
  return (
    <section
      className={cn('terminal-panel', secondary && 'terminal-panel-secondary')}
      aria-label={secondary ? '第二终端面板' : '终端面板'}
    >
      <div className="terminal-address">
        <div className="terminal-connection">
          <ShieldCheck size={13} />
          <span>
            {connection.user ? `${connection.user}@` : ''}{connection.host}
          </span>
          {connection.type === 'SSH' && <span className="terminal-port">:{connection.port}</span>}
          <Badge>{connection.type === 'Local' ? 'LOCAL' : 'SSH'}</Badge>
        </div>
        <div className="terminal-session-meta">
          <div className={cn('terminal-header-notice', manual && 'is-manual')}>
            {session.status === 'ended' ? <div><History size={13} /><span>会话已结束 · {session.result} · 终端输出已保留</span></div> : <>
              <div>
                {manual ? <Keyboard size={13} /> : <Radio size={13} />}
                <span>{manual ? '你正在控制终端 · AI 写入已暂停' : '允许 MCP 写入 · 你可以随时接管'}</span>
              </div>
            </>}
          </div>
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
              className={cn('terminal-control-option is-manual', manual && 'is-active')}
              aria-pressed={manual}
              disabled={session.status === 'ended'}
              onClick={() => setManual(true)}
            >
              <Hand size={12} />
              手动控制
            </button>
          </div>
          <span className={cn('terminal-ready', session.status === 'ended' && 'is-ended', session.status === 'attention' && 'is-attention')}>
            <span className={cn('status-dot', session.status === 'ended' && 'is-offline')} />
            {sessionStatus[session.status].label}
          </span>
          <span className="terminal-encoding">UTF-8</span>
          <Menu
            trigger={
              <Button variant="ghost" size="icon" aria-label="终端选项">
                <Ellipsis size={16} />
              </Button>
            }
            items={[
              {
                label: split ? '关闭分屏' : '拆分终端',
                icon: <SquareSplitHorizontal size={14} />,
                onSelect: () => { if (split || sessions.some(item => item.id !== session.id && item.status !== 'ended')) setSplit(!split); else notify('再打开一个会话即可拆分终端') },
              },
              {
                label: '查看终端历史',
                icon: <History size={14} />,
                onSelect: () => inspectHistory(session.id),
              },
            ]}
          />
        </div>
      </div>
      <div className="terminal-frame">
        <div className="terminal-body"><TerminalContent session={session} /></div>
      </div>
    </section>
  )
}

export default function Workspace() {
  const { selectedSession, sessions, split, openModal, connections, openConnection, openSession, navigate } = useDesign()
  const otherSession = sessions.find(item => item.id !== selectedSession.id && item.status !== 'ended')
  const profiles = connections.filter(connection => !connection.archived)
  const active = sessions.filter(session => session.status !== 'ended')
  if (!selectedSession.id) return <div className="workspace-page workspace-welcome">
    <div className="welcome-heading"><span className="welcome-kicker"><span className="gradient-line" />YOUR TERMINAL, CONNECTED</span><span className="welcome-badge"><ShieldCheck size={12} />本地工作区</span></div>
    <div className="welcome-content">
      <span className="welcome-mark"><Terminal size={32} strokeWidth={1.8} /></span>
      <h1>连接终端，<span>专注当下。</span></h1>
      <p>从一台主机、一个本地终端开始。<br />与 AI 共享工作区，每一步都由你掌控。</p>
      <div className="welcome-actions"><Button onClick={() => openModal(profiles.length ? 'session' : 'connection')}><Plus size={15} />{profiles.length ? '新建会话' : '添加第一个连接'}<ArrowRight size={14} /></Button><Button variant="outline" onClick={() => navigate('integrations')}><Workflow size={14} />配置 AI 接入</Button></div>
      <div className="welcome-quick-section"><div className="welcome-section-label"><span>{active.length ? '继续工作' : profiles.length ? '快速连接' : '从这里开始'}</span><span>{active.length ? `${active.length} 个活跃会话` : 'SSH / LOCAL / MCP'}</span></div>
        <div className="welcome-quick-grid">{active.length ? active.slice(0, 3).map(session => <button key={session.id} onClick={() => openSession(session.id)}><Terminal size={17} /><strong>{session.name}</strong><span>{session.mode} · {sessionStatus[session.status].label}</span><ArrowRight size={13} /></button>) : profiles.length ? profiles.slice(0, 3).map(connection => <button key={connection.id} onClick={() => openConnection(connection.id)}>{connection.type === 'Local' ? <Terminal size={17} /> : <Server size={17} />}<strong>{connection.name}</strong><span>{connection.type === 'Local' ? connection.command : connection.host}</span><ArrowRight size={13} /></button>) : <>
          <button onClick={() => openModal('connection')}><Server size={17} /><strong>管理连接</strong><span>SSH 主机与本地 Shell</span><ArrowRight size={13} /></button>
          <button onClick={() => navigate('integrations')}><Bot size={17} /><strong>接入 AI 工具</strong><span>Claude Code、Codex、Cursor</span><ArrowRight size={13} /></button>
          <button onClick={() => openModal('import')}><PanelsTopLeft size={17} /><strong>导入工作配置</strong><span>带上你熟悉的连接</span><ArrowRight size={13} /></button>
        </>}</div>
      </div>
    </div>
    <div className="welcome-footer"><span><Keyboard size={12} /><kbd>⌘ K</kbd>快速访问会话与连接</span><span><ShieldCheck size={12} />所有连接与记录保存在本机</span></div>
  </div>
  return <div className="workspace-page workspace-terminal-page"><div className="workspace-grid"><div className="workspace-main"><div className={cn('terminal-panes', split && otherSession && 'is-split')}><TerminalPanel session={selectedSession} />{split && otherSession && <TerminalPanel session={otherSession} secondary />}</div></div></div></div>
}
