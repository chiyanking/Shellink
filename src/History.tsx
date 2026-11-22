import { useEffect, useMemo, useState } from 'react'
import { Archive, ArrowRight, BellRing, FileClock, ListChecks, Search, Terminal, Trash2 } from 'lucide-react'
import { rpc, type HistoryChunk } from './backend'
import { useDesign } from './context'
import { sessionStatus, shortTime } from './data'
import { cn } from './lib/utils'
import { Badge, Button, Hint, Input } from './components/ui'
import { PageHeading } from './Workspace'

type ChunkPage = { items: HistoryChunk[]; cursor: number; hasMore: boolean }

export default function HistoryPage() {
  const { sessions, historyScope, inspectSession, notify, deleteSessions, deleteSessionsDirect, deletingSessions } = useDesign()
  const [sessionId, setSessionId] = useState(historyScope.sessionId ?? 'all')
  const [items, setItems] = useState<HistoryChunk[]>([])
  const [cursor, setCursor] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const selectedSession = sessions.find(session => session.id === sessionId)
  const orderedSessions = useMemo(() => [...sessions].sort((a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0)), [sessions])
  const endedSessions = useMemo(() => orderedSessions.filter(session => session.status === 'ended'), [orderedSessions])
  const endedSelected = endedSessions.length > 0 && endedSessions.every(session => selectedIds.includes(session.id))
  const filtered = useMemo(() => items.filter(item => `${item.direction} ${item.plain} ${item.sessionId}`.toLowerCase().includes(search.toLowerCase())), [items, search])
  useEffect(() => { setSessionId(historyScope.sessionId ?? 'all') }, [historyScope.sessionId])
  useEffect(() => {
    let alive = true
    setItems([]); setCursor(0); setHasMore(false)
    if (sessionId === 'all') return () => { alive = false }
    setLoading(true)
    void rpc<ChunkPage>('sessions.historyChunks', { id: sessionId, since: 0, limit: 500 }).then(page => {
      if (alive) { setItems(page.items); setCursor(page.cursor); setHasMore(page.hasMore) }
    }).catch(error => { if (alive) notify(String(error)) }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [sessionId, notify])
  async function loadMore() {
    if (!selectedSession || loading || !hasMore) return
    setLoading(true)
    try {
      const page = await rpc<ChunkPage>('sessions.historyChunks', { id: sessionId, since: cursor, limit: 500 })
      setItems(current => [...current, ...page.items]); setCursor(page.cursor); setHasMore(page.hasMore)
    } catch (error) { notify(String(error)) } finally { setLoading(false) }
  }
  return <div className="standard-page history-page">
    <PageHeading eyebrow="SESSION STREAM" title="历史记录" description="按 session 查看后台保存的原始输入与合并终端输出。" />
    <div className="connection-stats">
      {[
        { value: orderedSessions.length, label: '全部会话', icon: FileClock, tone: '' },
        { value: orderedSessions.filter(session => session.status !== 'ended').length, label: '活跃会话', icon: Terminal, tone: 'green' },
        { value: orderedSessions.filter(session => session.status === 'attention').length, label: '待接管', icon: BellRing, tone: 'amber' },
        { value: endedSessions.length, label: '已结束', icon: Archive, tone: 'purple' },
      ].map(stat => <div className={`connection-stat tone-${stat.tone}`} key={stat.label}><div><strong>{stat.value.toString().padStart(2, '0')}</strong><stat.icon size={17} /></div><span>{stat.label}</span></div>)}
    </div>
    {selecting ? <div className="history-batch-bar">
      <span className="history-batch-count"><ListChecks size={14} />已选择 {selectedIds.length} / {endedSessions.length} 个已结束会话</span>
      <div className="history-batch-actions">
        <label className="context-select-all"><input type="checkbox" aria-label="全选已结束会话" checked={endedSelected} disabled={deletingSessions} onChange={event => setSelectedIds(event.target.checked ? endedSessions.map(session => session.id) : [])} />全选</label>
        <Button size="sm" variant="outline" disabled={deletingSessions} onClick={() => { setSelecting(false); setSelectedIds([]) }}>取消</Button>
        <Button size="sm" variant="danger" disabled={deletingSessions || !selectedIds.length} onClick={() => { deleteSessions(selectedIds); setSelecting(false); setSelectedIds([]) }}>{deletingSessions ? '删除中…' : `删除所选 (${selectedIds.length})`}</Button>
      </div>
    </div> : endedSessions.length > 0 && <div className="history-picker-bar">
      <button className="context-bulk-delete" disabled={deletingSessions} onClick={() => { setSelecting(true); setSelectedIds([]) }}>批量删除</button>
    </div>}
    <div className="history-session-picker">
      {orderedSessions.map(session => {
        const ended = session.status === 'ended'
        return <div className={cn('history-session-card-wrap', session.id === sessionId && 'is-active')} key={session.id}>
          <button
            className={cn('history-session-card', selecting && (ended ? 'is-selectable' : 'is-disabled'), selecting && selectedIds.includes(session.id) && 'is-checked')}
            onClick={() => {
              if (selecting) {
                if (session.status !== 'ended') return
                setSelectedIds(current => current.includes(session.id) ? current.filter(id => id !== session.id) : [...current, session.id])
                return
              }
              setSessionId(current => current === session.id ? 'all' : session.id)
            }}
            title={selecting ? (ended ? '点击选择该会话' : '仅已结束的会话可批量删除') : session.id === sessionId ? '再次点击取消选择' : `查看 ${session.name} 的终端历史`}
          >
            <div className="history-card-top">
              <span className={cn('history-glyph', ended ? 'is-ended' : session.status === 'attention' ? 'is-attention' : 'is-live')}><Terminal size={16} /></span>
              <div className="history-card-name">
                <div className="history-card-title">
                  <strong>{session.name}</strong>
                  <span className="history-mode-tag">{session.mode === 'AUTO' ? 'AI 自动' : '手动'}</span>
                </div>
                <small>{session.createdAt ? shortTime(session.createdAt) : session.created} · 会话 #…{session.id.slice(-6)}</small>
              </div>
            </div>
            <div className="history-card-bottom">
              <span className={cn('history-card-state', `is-${session.status}`)}>
                <span className="history-state-dot" />
                <span className="history-state-label">{sessionStatus[session.status].label}</span>
                {ended && <><span className="history-state-sep">·</span><span>已归档</span></>}
              </span>
              <span className="history-card-actions">
                {ended && <Hint label="删除会话（终端历史一并删除，不再确认）"><span
                  role="button"
                  tabIndex={0}
                  className="history-session-delete"
                  aria-label={`删除会话 ${session.name}`}
                  onClick={event => { event.stopPropagation(); deleteSessionsDirect([session.id]) }}
                  onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); deleteSessionsDirect([session.id]) } }}
                ><Trash2 size={13} /></span></Hint>}
                <span
                  role="button"
                  tabIndex={0}
                  className="history-session-open"
                  aria-label={`查看 ${session.name} 的会话详情`}
                  title="会话详情"
                  onClick={event => { event.stopPropagation(); inspectSession(session.id) }}
                  onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); event.stopPropagation(); inspectSession(session.id) } }}
                ><ArrowRight size={13} /></span>
              </span>
            </div>
          </button>
        </div>
      })}
      {!orderedSessions.length && <div className="empty-state"><FileClock size={26} /><h3>还没有任何会话</h3><p>创建会话后，终端历史会按 session 保存在这里。</p></div>}
    </div>
    {selectedSession && <>
      <div className="history-detail-bar">
        <div className="history-detail-title">
          <strong>{selectedSession.name}</strong>
          <Badge>{filtered.length} 条片段</Badge>
        </div>
        <label className="search-input history-detail-search"><Search size={14} /><Input aria-label="搜索历史片段" placeholder="搜索输入或终端输出…" value={search} onChange={event => setSearch(event.target.value)} /></label>
      </div>
      <div className="history-feed">{filtered.map(item => <article className="history-entry" key={item.id}><div className="history-entry-main"><time>{new Date(item.createdAt).toLocaleString()}</time><span className="history-source-icon"><FileClock size={15} /></span><div className="history-entry-content"><div><strong>{item.direction === 'input' ? '终端输入' : '终端输出'}</strong><Badge>来源未知</Badge><Badge>{item.direction}</Badge></div><pre className="ui-selectable">{item.plain}</pre><span><code>seq {item.seq}</code><span>·</span><code>{item.sessionId}</code></span></div></div></article>)}</div>
      {!filtered.length && <div className="empty-state"><FileClock size={26}/><h3>{loading ? '正在读取历史' : '暂无匹配的历史片段'}</h3><p>所选会话中没有符合当前搜索的片段。</p></div>}
      {hasMore && <div className="history-end"><Button variant="outline" disabled={loading} onClick={() => void loadMore()}>{loading ? '正在加载…' : '加载更早片段'}</Button></div>}
    </>}
  </div>
}
