import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api, wsUrl, type SessionSummary } from '../api'

/** When a session stopped being live: its close time, or its start time if unknown. */
function sessionEndedAt(s: SessionSummary): number {
  return s.closedAt ?? s.createdAt
}

/** Global session list + active count, kept fresh via a single /events WebSocket. */
export const useSessionsStore = defineStore('sessions', () => {
  const sessions = ref<SessionSummary[]>([])
  const loading = ref(false)
  const ready = ref(false)
  let ws: WebSocket | null = null
  let started = false

  const activeCount = computed(() => sessions.value.filter((s) => s.active).length)
  const activeSessions = computed(() => sessions.value.filter((s) => s.active))
  // Newest first: "recently closed" must list the most recent session on top.
  // The API already returns active-first/newest-first, but sorting explicitly
  // keeps the order correct for cached or out-of-order responses and does not
  // depend on the server ordering.
  const closedSessions = computed(() =>
    sessions.value
      .filter((s) => !s.active)
      .slice()
      .sort((a, b) => sessionEndedAt(b) - sessionEndedAt(a))
      .slice(0, 20),
  )

  function stateOf(id: string): string | undefined {
    return sessions.value.find((s) => s.id === id)?.state
  }

  async function refresh() {
    loading.value = true
    try {
      sessions.value = await api<SessionSummary[]>('/sessions')
      ready.value = true
    } finally {
      loading.value = false
    }
  }

  function connectEvents() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return
    }
    ws = new WebSocket(wsUrl('/events'))
    ws.onmessage = () => {
      void refresh()
    }
    ws.onclose = () => {
      ws = null
      // Reconnect after a short delay if the store is still active.
      if (started) {
        window.setTimeout(() => {
          if (started) connectEvents()
        }, 2000)
      }
    }
  }

  async function init() {
    if (started) return
    started = true
    await refresh()
    connectEvents()
  }

  function dispose() {
    started = false
    ws?.close()
    ws = null
  }

  return {
    sessions,
    loading,
    ready,
    activeCount,
    activeSessions,
    closedSessions,
    stateOf,
    refresh,
    init,
    dispose,
  }
})
