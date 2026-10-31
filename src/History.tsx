import { useEffect, useMemo, useState } from 'react'
import { ArrowDownToLine, ArrowUpRight, FileClock, Search } from 'lucide-react'
import { rpc, type HistoryChunk } from './backend'
import { useDesign } from './context'
import { Badge, Button, Input, Select } from './components/ui'
import { PageHeading } from './Workspace'

type ChunkPage = { items: HistoryChunk[]; cursor: number; hasMore: boolean }

export function SessionHistoryPreview({ sessionId }: { sessionId: string }) {
  const { inspectHistory } = useDesign()
  const [items, setItems] = useState<HistoryChunk[]>([])
  useEffect(() => { let alive = true; void rpc<ChunkPage>('sessions.historyChunks', {id:sessionId,since:0,limit:40}).then(page => {if(alive)setItems(page.items.slice(-4).reverse())}).catch(() => {}); return () => {alive=false} }, [sessionId])
  return <div className="session-history-preview"><p className="review-caption">终端流不包含操作者身份或退出码。</p>{items.map(item => <div className="session-history-item" key={item.id}><div className="activity-content"><span className="session-history-meta"><strong>来源未知 · {item.direction}</strong><time>{new Date(item.createdAt).toLocaleTimeString()}</time></span><pre>{item.plain}</pre></div></div>)}{!items.length && <p className="review-caption">暂无历史片段。</p>}<Button variant="outline" className="session-history-open" onClick={() => inspectHistory(sessionId)}><FileClock size={13}/>查看全部终端历史<ArrowUpRight size={12}/></Button></div>
}

export default function HistoryPage() {
  const { sessions, connections, historyScope, inspectSession, notify } = useDesign()
  const [sessionId, setSessionId] = useState(historyScope.sessionId ?? 'all')
  const [items, setItems] = useState<HistoryChunk[]>([])
  const [cursor, setCursor] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const selectedSession = sessions.find(session => session.id === sessionId)
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
    <PageHeading eyebrow="SESSION STREAM" title="历史记录" description="按 session 查看后台保存的原始输入与合并终端输出。">
      <Button variant="outline" disabled={!selectedSession || !items.length} onClick={() => {
        const blob = new Blob([JSON.stringify({ sessionId, chunks: items }, null, 2)], { type: 'application/json' })
        const link = document.createElement('a'); link.href = URL.createObjectURL(blob); link.download = `shellink-${sessionId}-history.json`; link.click(); URL.revokeObjectURL(link.href)
      }}><ArrowDownToLine size={14} />导出 JSON</Button>
    </PageHeading>
    <div className="history-session-scope">
      <span className="history-source-icon"><FileClock size={18} /></span>
      <div><strong>{selectedSession?.name ?? '选择一个会话'}</strong><span>{selectedSession ? `${sessionId} · ${connections.find(item => item.id === selectedSession.connectionId)?.name ?? '已删除连接'}` : '历史按 session 分开保存'}</span></div>
      <Select label="历史记录所属会话" value={sessionId} onChange={setSessionId} options={[{ value: 'all', label: '选择会话' }, ...sessions.map(session => ({ value: session.id, label: `${session.name} · ${session.id}` }))]} />
      {selectedSession && <Button variant="ghost" size="sm" onClick={() => inspectSession(selectedSession.id)}>会话详情</Button>}
    </div>
    <div className="history-toolbar"><label className="search-input"><Search size={14} /><Input aria-label="搜索历史片段" placeholder="搜索输入或终端输出…" value={search} onChange={event => setSearch(event.target.value)} /></label></div>
    <div className="history-date"><span>{selectedSession ? selectedSession.name : '未选择会话'}</span><span>操作者和独立 stdout/stderr 不在终端流中记录</span><Badge>{filtered.length} 条片段</Badge></div>
    {sessionId !== 'all' && <div className="history-feed">{filtered.map(item => <article className="history-entry" key={item.id}><div className="history-entry-main"><time>{new Date(item.createdAt).toLocaleString()}</time><span className="history-source-icon"><FileClock size={15} /></span><div className="history-entry-content"><div><strong>{item.direction === 'input' ? '终端输入' : '终端输出'}</strong><Badge>来源未知</Badge><Badge>{item.direction}</Badge></div><pre className="ui-selectable">{item.plain}</pre><span><code>seq {item.seq}</code><span>·</span><code>{item.sessionId}</code></span></div></div></article>)}</div>}
    {sessionId === 'all' ? <div className="empty-state"><FileClock size={26}/><h3>选择会话查看终端历史</h3><p>该后台保存的是原始输入和合并后的 PTY 流。</p></div> : !filtered.length && <div className="empty-state"><FileClock size={26}/><h3>{loading ? '正在读取历史' : '暂无匹配的历史片段'}</h3><p>所选会话中没有符合当前搜索的片段。</p></div>}
    {hasMore && <div className="history-end"><Button variant="outline" disabled={loading} onClick={() => void loadMore()}>{loading ? '正在加载…' : '加载更早片段'}</Button></div>}
  </div>
}
