import { invoke, isTauri } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'

export type Profile = { id: string; name: string; connectType: 'ssh' | 'command'; host: string; port: number; username: string; authType: 'password' | 'key'; command: string | null; hasPassword: boolean; hasPrivateKey: boolean; term: string; cols: number; rows: number }
export type SessionRecord = { id: string; profileId: string; profileName: string; name: string; source: string; target: string; state: string; mode: 'AUTO' | 'MANUAL'; createdAt: number; closedAt?: number; closeReason?: string; active: boolean }
export type RuntimeInfo = { status: { version: string; pid: number; httpEnabled: boolean; httpUrl: string | null; activeSessions: number }; dataHome: string; logPath: string; command: string; defaultShell: string; platform: string }
export type MCPInfo = { enabled: boolean; running: boolean; httpEnabled: boolean; url: string; token: string; command: string; clientConfig: object; clientConfigStdio: object }
export type CallbackRecord = { id: string; receivedAt: number; data: unknown }
export type HistoryChunk = { id: number; seq: number; sessionId: string; direction: 'input' | 'output'; plain: string; createdAt: number }
export type CoreEvent = { subscriptionId: string; event: string; data: { sessionId?: string; direction?: string; raw?: string; plain?: string; seq?: number; [key: string]: unknown }; seq?: number }
export function rpc<T = unknown>(method: string, params: object = {}, timeoutMs = 30000): Promise<T> {
  if (!isTauri()) return Promise.reject(new Error('请使用 Shellink 桌面应用连接后台。可运行 npm run desktop:dev 打开原生窗口。'))
  return invoke<T>('core_request', { method, params, timeoutMs })
}
export const onCoreEvent = (callback: (event: CoreEvent) => void) => listen<CoreEvent>('shellink:event', event => callback(event.payload))
export const onDisconnect = (callback: (reason: string) => void) => listen<string>('shellink:disconnected', event => callback(event.payload))
export async function reconnect() { if (isTauri()) await invoke('core_reconnect') }
export function openDirectory(path: string) { return invoke('open_data_directory', { path }) }
export function errorText(error: unknown) { return error instanceof Error ? error.message : String(error) }
export function downloadJSON(name: string, data: unknown) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }))
  const a = document.createElement('a'); a.href = url; a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
export function preference<T>(key: string, fallback: T): T { try { return JSON.parse(localStorage.getItem(`shellink.${key}`) ?? 'null') ?? fallback } catch { return fallback } }
export function savePreference(key: string, value: unknown) { localStorage.setItem(`shellink.${key}`, JSON.stringify(value)) }
