import { ShellinkHttpError } from './errors.js'

type Params = Record<string, unknown>
type FetchLike = typeof fetch

export class ShellinkHttpClient {
  private closed = false

  constructor(
    private readonly baseUrl: string,
    private readonly defaultTimeoutMs = 30_000,
    private readonly fetchImpl: FetchLike = fetch,
  ) {}

  async request<T = unknown>(method: string, rawParams: unknown = {}, timeoutMs = this.defaultTimeoutMs): Promise<T> {
    if (this.closed) throw new Error('Shellink HTTP client is closed')
    const params = (rawParams ?? {}) as Params
    const request = this.toRequest(method, params)
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    try {
      const response = await this.fetchImpl(`${this.baseUrl}${request.path}`, {
        method: request.method,
        headers: request.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
        body: request.body instanceof FormData ? request.body : request.body === undefined ? undefined : JSON.stringify(request.body),
        signal: controller.signal,
      })
      const text = await response.text()
      let result: unknown
      if (text) {
        try {
          result = JSON.parse(text)
        } catch {
          result = text
        }
      }
      if (!response.ok) {
        const payload = result as { error?: unknown } | undefined
        throw new ShellinkHttpError(response.status, typeof payload?.error === 'string' ? payload.error : response.statusText)
      }
      if (response.status === 204) return { ok: true } as T
      return result as T
    } finally {
      clearTimeout(timer)
    }
  }

  close(): void {
    this.closed = true
  }

  private toRequest(method: string, params: Params): { method: string; path: string; body?: unknown } {
    const id = encodeURIComponent(String(params.id ?? ''))
    switch (method) {
      case 'profiles.list':
        return { method: 'GET', path: `/shellink/api/profiles${params.q ? `?q=${encodeURIComponent(String(params.q))}` : ''}` }
      case 'profiles.get':
        return { method: 'GET', path: `/shellink/api/profiles/${id}` }
      case 'profiles.create':
        return { method: 'POST', path: '/shellink/api/profiles', body: params }
      case 'profiles.update':
        return { method: 'PUT', path: `/shellink/api/profiles/${id}`, body: params.profile }
      case 'profiles.delete':
        return { method: 'DELETE', path: `/shellink/api/profiles/${id}` }
      case 'sessions.list':
        return { method: 'GET', path: '/shellink/api/sessions' }
      case 'sessions.create':
        return { method: 'POST', path: '/shellink/api/sessions', body: params }
      case 'sessions.state':
        return { method: 'GET', path: `/shellink/api/sessions/${id}/state` }
      case 'sessions.history': {
        const query = new URLSearchParams()
        if (params.since !== undefined) query.set('since', String(params.since))
        if (params.limit !== undefined) query.set('limit', String(params.limit))
        return { method: 'GET', path: `/shellink/api/sessions/${id}/history${query.size ? `?${query}` : ''}` }
      }
      case 'sessions.input':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/input`, body: omit(params, 'id') }
      case 'sessions.execStart':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/exec-start`, body: omit(params, 'id') }
      case 'sessions.execStatus':
        return { method: 'POST', path: `/shellink/api/sessions/${encodeURIComponent(String(params.jobId ?? ''))}/exec-status`, body: params }
      case 'sessions.execCancel':
        return { method: 'POST', path: `/shellink/api/sessions/${encodeURIComponent(String(params.jobId ?? ''))}/exec-cancel`, body: params }
      case 'sessions.mode':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/mode`, body: omit(params, 'id') }
      case 'sessions.close':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/close` }
      case 'sessions.uploadStart': {
        const query = new URLSearchParams({ path: String(params.path ?? '') })
        if (params.timeoutMs !== undefined) query.set('timeoutMs', String(params.timeoutMs))
        const form = new FormData()
        const bytes = Uint8Array.from(params.data as Uint8Array)
        form.append('file', new Blob([bytes.buffer as ArrayBuffer]), 'upload.bin')
        return { method: 'POST', path: `/shellink/api/sessions/${id}/upload-start?${query}`, body: form }
      }
      case 'sessions.downloadStart':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/download-start`, body: omit(params, 'id') }
      case 'sessions.editStart':
        return { method: 'POST', path: `/shellink/api/sessions/${id}/edit-start`, body: omit(params, 'id') }
      default:
        throw new Error(`Unsupported Shellink API method: ${method}`)
    }
  }
}

function omit(params: Params, key: string): Params {
  const { [key]: _ignored, ...rest } = params
  return rest
}
