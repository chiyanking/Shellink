import { useMemo, useState } from 'react'
import { ArrowUpRight, Braces, Copy, Inbox, Link2, RefreshCw, Search, Trash2, Webhook } from 'lucide-react'
import { rpc } from './backend'
import { useDesign } from './context'
import { Badge, Button, Dialog, Input, Select } from './components/ui'
import { cn } from './lib/utils'

const displayTime = (value: number) => new Date(value).toLocaleString()
const property = (value: unknown, key: string) => value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>)[key] : undefined

export default function WebhookMessages() {
  const { messages, runtime, refresh, notify, inspectHistory } = useDesign()
  const [search, setSearch] = useState('')
  const [eventType, setEventType] = useState('all')
  const [selectedId, setSelectedId] = useState('')
  const [clearOpen, setClearOpen] = useState(false)
  const [pretty, setPretty] = useState(true)
  const [busy, setBusy] = useState(false)
  const endpoint = runtime?.status.httpEnabled && runtime.status.httpUrl ? `${runtime.status.httpUrl.replace(/\/$/, '')}/shellink/webhook/callback` : ''
  const eventName = (data: unknown) => typeof property(data, 'event') === 'string' ? String(property(data, 'event')) : '未提供事件名'
  const shown = useMemo(() => messages.filter(message => (eventType === 'all' || eventName(message.data) === eventType) && JSON.stringify(message.data).toLowerCase().includes(search.toLowerCase())), [messages, eventType, search])
  const selected = shown.find(message => message.id === selectedId) ?? shown[0]
  const body = selected ? JSON.stringify(selected.data, null, pretty ? 2 : undefined) : ''
  const sessionId = typeof property(selected?.data, 'sessionId') === 'string' ? String(property(selected?.data, 'sessionId')) : undefined
  async function copy(value: string) { try { await navigator.clipboard.writeText(value); notify('已复制到剪贴板') } catch { notify('复制未完成，可选中内容后复制。') } }
  async function clear() { setBusy(true); try { await rpc('webhooks.clearMessages'); await refresh(); setSelectedId(''); setClearOpen(false) } catch (error) { notify(String(error)) } finally { setBusy(false) } }
  async function refreshMessages() { setBusy(true); try { await refresh() } finally { setBusy(false) } }
  return <section className="webhook-page" aria-label="Webhook 回调消息">
    <div className="webhook-heading"><div><span className="eyebrow">INCOMING WEBHOOKS</span><h2>Webhook 回调消息</h2><p>查看后台接收的 HTTP 回调原文。</p></div><Badge>{messages.length} 条</Badge></div>
    <div className="webhook-endpoint"><div><span className="review-actor-icon actor-ai"><Webhook size={18}/></span><div><strong>回调接收地址</strong><span>POST · payload 由发送方定义</span></div><Badge tone={endpoint ? 'green' : 'neutral'}>{endpoint ? '可用' : '不可用'}</Badge></div><div className="webhook-endpoint-url"><Link2 size={13}/><code className="ui-selectable">{endpoint || 'HTTP 服务未启用'}</code><Button variant="outline" size="sm" disabled={!endpoint} onClick={() => void copy(endpoint)}><Copy size={12}/>复制地址</Button></div></div>
    <div className="webhook-message-heading"><div><Inbox size={15}/><h3>收到的消息</h3><Badge>{messages.length}</Badge></div><div><Button variant="outline" size="sm" disabled={busy} onClick={() => void refreshMessages()}><RefreshCw size={12}/>刷新</Button><Button variant="ghost" size="sm" disabled={!messages.length || busy} onClick={() => setClearOpen(true)}><Trash2 size={12}/>清空</Button></div></div>
    <div className="webhook-toolbar"><label className="search-input"><Search size={13}/><Input aria-label="搜索回调消息" placeholder="搜索消息内容…" value={search} onChange={event => setSearch(event.target.value)}/></label><Select label="回调事件筛选" value={eventType} onChange={setEventType} options={[{value:'all',label:'全部事件'}, ...[...new Set(messages.map(message => eventName(message.data)))].map(value => ({value,label:value}))]}/></div>
    {selected ? <div className="webhook-message-layout"><div className="webhook-message-list" aria-label="收到的回调列表">{shown.map(message => <button key={message.id} aria-pressed={selected.id === message.id} className={cn('webhook-message-row', selected.id === message.id && 'is-selected')} onClick={() => setSelectedId(message.id)}><span className="webhook-event-dot"/><span><strong>{eventName(message.data)}</strong><span>{String(property(message.data, 'source') ?? '未提供来源')}</span><time>{displayTime(message.receivedAt)}</time></span></button>)}</div><div className="webhook-message-detail"><div className="review-panel-heading"><span><Braces size={13}/>消息原文</span><Badge>JSON</Badge></div><dl className="webhook-message-metadata"><div><dt>事件</dt><dd>{eventName(selected.data)}</dd></div><div><dt>接收时间</dt><dd>{displayTime(selected.receivedAt)}</dd></div><div><dt>消息 ID</dt><dd><code>{selected.id}</code></dd></div></dl>{sessionId && <button className="webhook-related-session text-link" onClick={() => inspectHistory(sessionId)}>查看关联会话历史<ArrowUpRight size={12}/><code>{sessionId}</code></button>}<div className="webhook-json-toolbar"><button className="text-link" onClick={() => setPretty(!pretty)}>{pretty ? '压缩显示' : '格式化显示'}</button><Button variant="ghost" size="sm" onClick={() => void copy(body)}><Copy size={12}/>复制 JSON</Button></div><pre className="webhook-json ui-selectable">{body}</pre></div></div> : <div className="empty-state review-empty"><Inbox size={29}/><h3>{messages.length ? '没有匹配的回调消息' : '暂无收到的回调消息'}</h3><p>{messages.length ? '调整搜索或事件筛选。' : '收到的事件将在这里展示。'}</p><Button variant="outline" onClick={() => {setSearch('');setEventType('all')}}>重置筛选</Button></div>}
    <div className="webhook-footer"><span>显示 {shown.length} 条 · 时间为本地显示</span><span>事件与来源字段由发送方提供</span></div>
    <Dialog open={clearOpen} onOpenChange={setClearOpen} title="清空收到的消息？" description={`将从后台移除 ${messages.length} 条回调消息。`}><div className="dialog-footer"><Button variant="outline" onClick={() => setClearOpen(false)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void clear()}>{busy ? '正在清空…' : '清空消息'}</Button></div></Dialog>
  </section>
}
