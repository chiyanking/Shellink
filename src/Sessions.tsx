import { useState } from 'react'
import {
  ArrowRight,
  ArrowUpRight,
  Bot,
  Check,
  ChevronRight,
  CircleHelp,
  Clock3,
  Copy,
  FileClock,
  Hand,
  Layers2,
  MoreHorizontal,
  PanelTop,
  Pencil,
  Plus,
  Search,
  Server,
  Square,
  Terminal,
  Trash2,
  UserRound,
  X,
} from 'lucide-react'
import { useDesign } from './context'
import { emptyConnection, emptySession, sessionStatus, type Session } from './data'
import { Badge, Button, Dialog, Hint, Input, Menu, Select } from './components/ui'
import { cn } from './lib/utils'
import { PageHeading } from './Workspace'
import './sessions.css'
import { SessionHistoryPreview } from './History'

function SourceIcon({ source, size = 13 }: { source: Session['source']; size?: number }) {
  return source === 'MCP' ? (
    <Bot size={size} />
  ) : (
    <UserRound size={size} />
  )
}

export default function SessionsPage() {
  const {
    sessions,
    connections,
    tabs,
    openSession,
    openModal,
    inspectedSessionId,
    inspectSession,
    endSessions,
    removeSessions,
    renameSession,
    setSessionManual,
    notify,
  } = useDesign()
  const [filter, setFilter] = useState(() =>
    sessions.find((session) => session.id === inspectedSessionId)?.status === 'ended'
      ? 'ended'
      : 'active',
  )
  const [search, setSearch] = useState('')
  const [source, setSource] = useState('all')
  const [checked, setChecked] = useState<string[]>([])
  const [ending, setEnding] = useState<string[]>([])
  const [deleting, setDeleting] = useState<string[]>([])
  const [renaming, setRenaming] = useState<Session | null>(null)
  const [name, setName] = useState('')
  const [detailTab, setDetailTab] = useState<'overview' | 'record'>('overview')
  const active = sessions.filter((session) => session.status !== 'ended')
  const attention = active.filter((session) => session.status === 'attention')
  const shown = sessions.filter((session) => {
    const connection = connections.find((item) => item.id === session.connectionId) ?? emptyConnection
    return (
      (filter === 'all' ||
        (filter === 'active'
          ? session.status !== 'ended'
          : filter === 'attention'
            ? session.status === 'attention'
            : session.status === 'ended')) &&
      (source === 'all' || session.source === source) &&
      `${session.name} ${session.id} ${connection.name} ${connection.host} ${session.client}`
        .toLowerCase()
        .includes(search.toLowerCase())
    )
  })
  const selected =
    shown.find((session) => session.id === inspectedSessionId) ??
    shown[0] ??
    sessions.find((session) => session.id === inspectedSessionId) ??
    sessions[0] ?? emptySession
  const host = connections.find((connection) => connection.id === selected.connectionId) ?? emptyConnection
  const selectedActive = selected.status !== 'ended'
  const checkedSessions = sessions.filter((session) => checked.includes(session.id))
  const checkedActive = checkedSessions.filter((session) => session.status !== 'ended')
  const allChecked =
    shown.length > 0 && shown.every((session) => checked.includes(session.id))
  const endingSessions = sessions.filter((session) => ending.includes(session.id))
  const endingRunning = endingSessions.some((session) => session.status === 'running')
  const deletingSessions = sessions.filter((session) => deleting.includes(session.id))

  function selectSession(id: string, record = false) {
    inspectSession(id)
    setDetailTab(record ? 'record' : 'overview')
  }
  function toggleChecked(id: string) {
    setChecked((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id],
    )
  }
  function startRename(session: Session) {
    setRenaming(session)
    setName(session.name)
  }

  return (
    <div className="standard-page sessions-page">
      <PageHeading
        eyebrow="SESSIONS"
        title="会话管理"
        description="每一个终端实例，都能在这里找到。界面与 MCP 统一管理。"
      >
        <Button onClick={() => openModal('session')}>
          <Plus size={15} />
          新建会话
        </Button>
      </PageHeading>

      <div className="session-summary-strip">
        <div className="session-summary-icon">
          <Layers2 size={20} />
        </div>
        <div className="session-summary-metric">
          <strong>{active.length}</strong>
          <span>活跃会话</span>
        </div>
        <span className="session-summary-divider" />
        <div className="session-summary-secondary">
          <PanelTop size={14} />
          <strong>{tabs.length}</strong> 在工作台打开
        </div>
        <div className="session-summary-secondary">
          <Terminal size={14} />
          <strong>{active.filter((session) => !tabs.includes(session.id)).length}</strong>{' '}
          在后台保留
        </div>
        <Hint label="工作台标签只是会话的一个视图。MCP 创建的会话也会出现在此处。">
          <button className="session-summary-help" aria-label="会话与标签的区别">
            <CircleHelp size={15} />
          </button>
        </Hint>
      </div>

      {attention.length > 0 && (
        <div className="session-attention-banner">
          <span className="session-attention-icon">
            <Hand size={15} />
          </span>
          <div>
            <strong>{attention.length} 个会话需要你接管</strong>
            <span>{attention[0]!.name} · 等待输入 sudo 密码，AI 写入已暂停</span>
          </div>
          <button
            onClick={() => {
              setFilter('attention')
              setSource('all')
              setSearch('')
              selectSession(attention[0]!.id)
            }}
          >
            查看会话
            <ArrowRight size={13} />
          </button>
        </div>
      )}

      <div className="sessions-layout">
        <section className="sessions-list" aria-label="全部终端会话">
          <div className="session-filter-tabs filter-tabs" aria-label="会话状态筛选">
            {[
              { id: 'active', label: '活跃', count: active.length },
              { id: 'attention', label: '待接管', count: attention.length },
              { id: 'ended', label: '已结束', count: sessions.length - active.length },
              { id: 'all', label: '全部', count: sessions.length },
            ].map((tab) => (
              <button
                key={tab.id}
                aria-pressed={filter === tab.id}
                className={cn(filter === tab.id && 'is-active')}
                onClick={() => setFilter(tab.id)}
              >
                {tab.label}
                <span>{tab.count}</span>
              </button>
            ))}
          </div>
          <div className="session-list-toolbar">
            <label className="search-input">
              <Search size={14} />
              <Input
                aria-label="搜索会话"
                placeholder="搜索名称、主机、会话 ID…"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
            <Select
              label="会话创建来源"
              value={source}
              onChange={setSource}
              options={[
                { value: 'all', label: '全部来源' },
                { value: 'MCP', label: 'MCP 创建' },
                { value: '你', label: '界面创建' },
              ]}
            />
          </div>

          {checkedSessions.length > 0 && (
            <div className="session-bulk-bar">
              <button aria-label="取消选择会话" onClick={() => setChecked([])}>
                <X size={14} />
              </button>
              <span>已选 {checkedSessions.length} 个会话</span>
              {checkedActive.length > 0 && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setEnding(checkedActive.map((session) => session.id))}
                >
                  <Square size={12} />
                  结束活跃会话
                </Button>
              )}
              <Button
                variant="danger"
                size="sm"
                onClick={() => setDeleting(checkedSessions.map((session) => session.id))}
              >
                <Trash2 size={12} />
                删除会话
              </Button>
            </div>
          )}

          <div className="session-table-wrap">
            <table className="session-manager-table">
              <thead>
                <tr>
                  <th className="session-check-cell">
                    <input
                      type="checkbox"
                      aria-label="选择当前列表的会话"
                      checked={allChecked}
                      disabled={!shown.length}
                      onChange={() =>
                        setChecked(
                          allChecked
                            ? checked.filter(
                                (id) => !shown.some((session) => session.id === id),
                              )
                            : [
                                ...new Set([
                                  ...checked,
                                  ...shown.map((session) => session.id),
                                ]),
                              ],
                        )
                      }
                    />
                  </th>
                  <th>会话 / 连接</th>
                  <th>状态</th>
                  <th>创建来源 / 控制权</th>
                  <th className="session-updated-column">最近活动</th>
                  <th>
                    <span className="sr-only">操作</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {shown.map((session) => {
                  const connection = connections.find((item) => item.id === session.connectionId)!
                  const attached = tabs.includes(session.id)
                  return (
                    <tr
                      key={session.id}
                      className={cn(
                        selected.id === session.id && 'is-selected',
                        session.status === 'ended' && 'is-ended',
                      )}
                    >
                      <td className="session-check-cell">
                        <input
                          type="checkbox"
                          aria-label={`选择 ${session.name}`}
                          checked={checked.includes(session.id)}
                          onChange={() => toggleChecked(session.id)}
                        />
                      </td>
                      <td>
                        <button
                          className="session-row-identity"
                          onClick={() => selectSession(session.id)}
                          aria-pressed={selected.id === session.id}
                        >
                          <span className={cn('session-row-icon', `color-${connection.color}`)}>
                            <Terminal size={16} />
                          </span>
                          <span>
                            <strong>
                              {session.name}
                              {attached && (
                                <span className="session-attached" title="已在工作台打开">
                                  <PanelTop size={10} />
                                </span>
                              )}
                            </strong>
                            <small>
                              {connection.name}
                              <span>·</span>
                              <code>{session.id}</code>
                            </small>
                          </span>
                        </button>
                      </td>
                      <td>
                        <Badge
                          tone={sessionStatus[session.status].tone}
                          className={`session-state-${session.status}`}
                        >
                          <span className="session-state-dot" />
                          {sessionStatus[session.status].label}
                        </Badge>
                      </td>
                      <td>
                        <div className="session-owner-cell">
                          <span>
                            <SourceIcon source={session.source} />
                            {session.source === '你' ? '界面' : session.source}
                          </span>
                          <small>
                            {session.status === 'ended' ? (
                              '—'
                            ) : session.mode === 'MANUAL' ? (
                              <>
                                <Hand size={10} />
                                人工控制
                              </>
                            ) : (
                              <>
                                <Bot size={10} />
                                自动模式
                              </>
                            )}
                          </small>
                        </div>
                      </td>
                      <td className="session-updated-column">
                        <span className="session-updated">{session.updated}</span>
                      </td>
                      <td>
                        <Menu
                          trigger={
                            <Button
                              variant="ghost"
                              size="icon"
                              aria-label={`${session.name} 的会话操作`}
                            >
                              <MoreHorizontal size={16} />
                            </Button>
                          }
                          items={[
                            ...(session.status !== 'ended'
                              ? [
                                  {
                                    label: attached ? '切换到终端' : '在工作台打开',
                                    icon: <ArrowUpRight size={14} />,
                                    onSelect: () => openSession(session.id),
                                  },
                                  {
                                    label: '重命名会话',
                                    icon: <Pencil size={14} />,
                                    onSelect: () => startRename(session),
                                  },
                                ]
                              : []),
                            {
                              label: '查看操作记录',
                              icon: <FileClock size={14} />,
                              onSelect: () => selectSession(session.id, true),
                            },
                            {
                              label: '复制会话 ID',
                              icon: <Copy size={14} />,
                              onSelect: () => void navigator.clipboard.writeText(session.id).then(() => notify('已复制会话 ID')).catch(() => notify(session.id)),
                            },
                            {
                              label: '删除会话记录',
                              icon: <Trash2 size={14} />,
                              danger: true,
                              onSelect: () => setDeleting([session.id]),
                            },
                            ...(session.status !== 'ended'
                              ? [
                                  {
                                    label: '结束会话',
                                    icon: <Square size={14} />,
                                    danger: true,
                                    onSelect: () => setEnding([session.id]),
                                  },
                                ]
                              : []),
                          ]}
                        />
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            {!shown.length && (
              <div className="empty-state">
                <Search size={26} />
                <h3>没有匹配的会话</h3>
                <p>试试其他名称、来源或状态。</p>
                <Button
                  variant="outline"
                  onClick={() => {
                    setFilter('all')
                    setSource('all')
                    setSearch('')
                  }}
                >
                  重置筛选
                </Button>
              </div>
            )}
          </div>
          <div className="session-list-footer">
            <span>
              显示 {shown.length} / {sessions.length} 个会话
            </span>
            <span>
              <PanelTop size={11} />
              已在工作台打开
            </span>
          </div>
          <div className="session-lifecycle-note">
            <CircleHelp size={14} />
            <p>
              <strong>收起标签，会话继续保留。</strong>{' '}
              结束会话会关闭终端进程，已有操作记录仍可查看。
            </p>
          </div>
        </section>

        <aside className="session-detail-panel" aria-label="会话详情">
          {!selected.id ? <div className="empty-state"><Layers2 size={26}/><h3>暂无会话</h3><p>创建会话后可在这里查看状态与终端历史。</p><Button onClick={() => openModal('session')}><Plus size={14}/>新建会话</Button></div> : <>
          <div className="session-detail-eyebrow">
            <span>会话详情</span>
            <Badge tone={sessionStatus[selected.status].tone}>
              {sessionStatus[selected.status].label}
            </Badge>
          </div>
          <div className="session-detail-title">
            <span className={cn('connection-icon', `color-${host.color}`)}>
              <Terminal size={19} />
            </span>
            <div>
              <h2>{selected.name}</h2>
              <span>{host.name}</span>
            </div>
            {selectedActive && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="重命名当前会话"
                onClick={() => startRename(selected)}
              >
                <Pencil size={13} />
              </Button>
            )}
          </div>
          <button
            className="session-detail-id"
            onClick={() => void navigator.clipboard.writeText(selected.id).then(() => notify('已复制会话 ID')).catch(() => notify(selected.id))}
          >
            <code>{selected.id}</code>
            <Copy size={12} />
          </button>
          {selectedActive ? (
            <Button className="session-detail-open" onClick={() => openSession(selected.id)}>
              <Terminal size={14} />
              {tabs.includes(selected.id) ? '回到终端' : '在工作台打开'}
              <ArrowUpRight size={13} />
            </Button>
          ) : (
            <div className="session-ended-note">
              <FileClock size={15} />
              <span>会话已结束，操作记录已保留。</span>
            </div>
          )}

          <div className="session-detail-tabs" aria-label="会话详情视图">
            <button
              aria-pressed={detailTab === 'overview'}
              className={detailTab === 'overview' ? 'is-active' : ''}
              onClick={() => setDetailTab('overview')}
            >
              概览
            </button>
            <button
              aria-pressed={detailTab === 'record'}
              className={detailTab === 'record' ? 'is-active' : ''}
              onClick={() => setDetailTab('record')}
            >
              操作记录
            </button>
          </div>
          {detailTab === 'overview' ? (
            <>
              <dl className="session-metadata">
                <div>
                  <dt>连接目标</dt>
                  <dd>
                    <Server size={11} />
                    {host.user}@{host.host}
                  </dd>
                </div>
                <div>
                  <dt>创建来源</dt>
                  <dd>
                    <SourceIcon source={selected.source} size={12} />
                    {selected.client}
                    <Badge>{selected.source === '你' ? '界面' : selected.source}</Badge>
                  </dd>
                </div>
                <div>
                  <dt>创建时间</dt>
                  <dd>{selected.created}</dd>
                </div>
                <div>
                  <dt>会话时长</dt>
                  <dd>
                    <Clock3 size={11} />
                    {selected.duration}
                  </dd>
                </div>
                <div>
                  <dt>{selectedActive ? '工作台视图' : '结束时间'}</dt>
                  <dd>
                    {selectedActive
                      ? tabs.includes(selected.id)
                        ? '已打开标签'
                        : '后台保留 · 可随时打开'
                      : selected.endedAt}
                  </dd>
                </div>
              </dl>
              {selectedActive && (
                <div className="session-detail-control">
                  <div className="section-label">
                    控制权<code>{selected.mode}</code>
                  </div>
                  <div className="control-segments">
                    <button
                      className={selected.mode === 'AUTO' ? 'is-active' : ''}
                      aria-pressed={selected.mode === 'AUTO'}
                      onClick={() => setSessionManual(selected.id, false)}
                    >
                      <Bot size={13} />
                      AI 自动
                    </button>
                    <button
                      className={selected.mode === 'MANUAL' ? 'is-active' : ''}
                      aria-pressed={selected.mode === 'MANUAL'}
                      onClick={() => setSessionManual(selected.id, true)}
                    >
                      <Hand size={13} />
                      人工控制
                    </button>
                  </div>
                  <p>
                    {selected.status === 'attention'
                      ? '终端正在等待密码，需要你打开终端接管。'
                      : selected.mode === 'AUTO'
                        ? 'MCP 可向此会话提交操作。'
                        : 'AI 写入暂停，仍可观察终端输出。'}
                  </p>
                </div>
              )}
              <div className="session-last-command">
                <div className="section-label">
                  最近命令
                  <span>
                    {selected.status === 'running'
                      ? '执行中'
                      : selected.status === 'attention'
                        ? '等待输入'
                        : '已完成'}
                  </span>
                </div>
                <code className="ui-selectable">$ {selected.command}</code>
                <span>{selected.result}</span>
              </div>
            </>
          ) : (
            <SessionHistoryPreview key={selected.id} sessionId={selected.id} />
          )}
          {selectedActive && (
            <div className="session-detail-bottom">
              <Button variant="ghost" onClick={() => setEnding([selected.id])}>
                <Square size={12} />
                结束会话
              </Button>
              <span>
                保留操作记录
                <ChevronRight size={11} />
              </span>
            </div>
          )}
          </>}
        </aside>
      </div>

      <Dialog
        open={ending.length > 0}
        onOpenChange={(open) => !open && setEnding([])}
        title={ending.length > 1 ? `结束 ${ending.length} 个会话？` : '结束此会话？'}
        description="终端进程和正在执行的任务将停止。结束后无法继续输入，操作记录仍会保留。"
      >
        <div className="session-ending-list">
          {endingSessions.map((session) => (
            <div key={session.id}>
              <Terminal size={15} />
              <span>
                <strong>{session.name}</strong>
                <code>{session.id}</code>
              </span>
              <Badge tone={sessionStatus[session.status].tone}>
                {sessionStatus[session.status].label}
              </Badge>
            </div>
          ))}
        </div>
        {endingRunning && (
          <p className="session-ending-warning">
            <Clock3 size={14} />
            所选会话中有命令仍在执行。
          </p>
        )}
        <div className="session-ending-alternative">
          <PanelTop size={15} />
          <p>只是暂时不看终端？可以收起工作台标签，保留会话。</p>
        </div>
        <div className="dialog-footer">
          <span className="subtle">终端进程和任务将停止</span>
          <Button variant="outline" onClick={() => setEnding([])}>
            取消
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              endSessions(ending)
              setChecked((current) => current.filter((id) => !ending.includes(id)))
              setFilter('ended')
              setSource('all')
              setSearch('')
              selectSession(ending[0]!, true)
              setEnding([])
            }}
          >
            结束会话
          </Button>
        </div>
      </Dialog>
      <Dialog
        open={deleting.length > 0}
        onOpenChange={(open) => !open && setDeleting([])}
        title={deleting.length > 1 ? `删除 ${deleting.length} 个会话？` : '删除此会话？'}
        description="会话及其全部操作记录将被永久删除，无法恢复。活跃会话也会同时关闭。"
      >
        <div className="session-ending-list">
          {deletingSessions.map((session) => (
            <div key={session.id}>
              <Trash2 size={15} />
              <span>
                <strong>{session.name}</strong>
                <code>{session.id}</code>
              </span>
              <Badge tone={sessionStatus[session.status].tone}>
                {sessionStatus[session.status].label}
              </Badge>
            </div>
          ))}
        </div>
        <div className="dialog-footer">
          <span className="subtle">操作记录删除后无法恢复</span>
          <Button variant="outline" onClick={() => setDeleting([])}>
            取消
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              const ids = deleting
              void removeSessions(ids)
                .then(() => setChecked((current) => current.filter((id) => !ids.includes(id))))
                .catch(() => {})
              setDeleting([])
            }}
          >
            删除会话
          </Button>
        </div>
      </Dialog>
      <Dialog
        open={renaming !== null}
        onOpenChange={(open) => !open && setRenaming(null)}
        title="重命名会话"
        description="给终端实例一个容易辨认的名字，不影响连接配置。"
      >
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (!name.trim() || !renaming) return
            renameSession(renaming.id, name)
            setRenaming(null)
          }}
        >
          <label className="form-field">
            会话名称
            <Input
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={40}
              required
            />
          </label>
          <div className="dialog-footer">
            <Button variant="outline" onClick={() => setRenaming(null)}>
              取消
            </Button>
            <Button type="submit" disabled={!name.trim()}>
              <Check size={14} />
              保存名称
            </Button>
          </div>
        </form>
      </Dialog>
    </div>
  )
}
