import { useEffect, useMemo, useState } from 'react'
import { ArrowRight, FileClock, Info, Search } from 'lucide-react'
import { rpc, type HistoryChunk } from './backend'
import { useDesign } from './context'
import { sessionStatus, shortTime } from './data'
import { cn } from './lib/utils'
import { Badge, Button, Hint, Input } from './components/ui'
import { PageHeading } from './Workspace'

type ChunkPage = { items: HistoryChunk[]; cursor: number; hasMore: boolean }

export default function HistoryPage() {
  const { sessions, historyScope, inspectSession, notify } = useDesign()
  const [sessionId, setSessionId] = useState(historyScope.sessionId ?? 'all')
  const [items, setItems] = useState<HistoryChunk[]>([])
  const [cursor, setCursor] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const selectedSession = sessions.find(session => session.id === sessionId)
  const orderedSessions = useMemo(() => [...sessions].sort((a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0)), [sessions])
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
    <div className="history-session-picker">
      {orderedSessions.map(session => <div className={cn('history-session-card-wrap', session.id === sessionId && 'is-active')} key={session.id}>
        <button className="history-session-card" onClick={() => setSessionId(current => current === session.id ? 'all' : session.id)} title={session.id === sessionId ? '再次点击取消选择' : `查看 ${session.name} 的终端历史`}>
          <strong>{session.name}</strong>
          <small>{session.mode} · {session.createdAt ? shortTime(session.createdAt) : session.created} · {sessionStatus[session.status].label}</small>
          <ArrowRight size={13} />
        </button>
        <Hint label="查看会话详情"><span
          role="button"
          tabIndex={0}
          className="history-session-detail"
          aria-label={`查看 ${session.name} 的会话详情`}
          onClick={event => { event.stopPropagation(); inspectSession(session.id) }}
          onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); inspectSession(session.id) } }}
        ><Info size={13} /></span></Hint>
      </div>)}
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
