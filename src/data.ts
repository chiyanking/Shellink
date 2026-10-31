import type { Profile, SessionRecord } from './backend'
import { preference } from './backend'
export type Page = 'workspace' | 'sessions' | 'connections' | 'integrations' | 'history' | 'settings'
export type SessionStatus = 'ready' | 'running' | 'attention' | 'ended'
export type Session = { id: string; connectionId: string; name: string; status: SessionStatus; source: 'MCP' | '你' | '未知'; client: string; mode: 'AUTO' | 'MANUAL'; created: string; updated: string; duration: string; command: string; result: string; endedAt?: string; state?: string; createdAt?: number }
export type Connection = { id: string; name: string; host: string; user: string; group: string; type: 'SSH' | 'Local'; color: 'blue' | 'purple' | 'green'; connected: boolean; system: string; port: number; command?: string; profile?: Profile; archived?: boolean }
export const sessionStatus: Record<SessionStatus, { label: string; tone: 'green' | 'blue' | 'amber' | 'neutral' }> = { ready: { label: '等待输入', tone: 'green' }, running: { label: '执行中', tone: 'blue' }, attention: { label: '待接管', tone: 'amber' }, ended: { label: '已结束', tone: 'neutral' } }
export const emptyConnection: Connection = { id: '', name: '暂无连接', host: '', user: '', group: 'Local', type: 'Local', color: 'green', connected: false, system: '', port: 0 }
export const emptySession: Session = { id: '', connectionId: '', name: '暂无会话', status: 'ended', source: '未知', client: '未标记来源', mode: 'AUTO', created: '', updated: '', duration: '', command: '', result: '' }
export const dateText = (at: number) => new Date(at).toLocaleString('sv-SE')
export function connectionFrom(profile: Profile, sessions: SessionRecord[]): Connection {
  const local = profile.connectType === 'command'
  return { id: profile.id, name: profile.name, host: local ? 'localhost' : profile.host, user: local ? '' : profile.username, port: profile.port, group: preference(`group.${profile.id}`, local ? 'Local' : 'Development'), type: local ? 'Local' : 'SSH', color: local ? 'green' : 'blue', connected: sessions.some(s => s.profileId === profile.id && s.active), system: local ? profile.command ?? '' : 'SSH', command: profile.command ?? '', profile }
}
export function sessionFrom(s: SessionRecord): Session {
  const source = s.source === 'desktop' ? '你' : s.source === 'mcp' ? 'MCP' : '未知'
  const elapsed = Math.max(0, (s.closedAt ?? Date.now()) - s.createdAt)
  return { id: s.id, connectionId: s.profileId, name: s.name || s.profileName || s.id, status: !s.active ? 'ended' : s.state === 'CONNECTING' ? 'attention' : s.state === 'OUTPUTTING' ? 'running' : 'ready', source, client: source === '你' ? '桌面端创建' : source === '未知' ? '未标记来源' : `${source} 创建`, mode: s.mode, created: dateText(s.createdAt), updated: dateText(s.closedAt ?? s.createdAt), duration: elapsed < 60000 ? `${Math.floor(elapsed / 1000)} 秒` : `${Math.floor(elapsed / 60000)} 分钟`, command: '查看终端历史', result: s.closeReason ?? s.state, endedAt: s.closedAt ? dateText(s.closedAt) : undefined, state: s.state, createdAt: s.createdAt }
}
