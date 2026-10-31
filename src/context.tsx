import { createContext, useContext } from 'react'
import type { Connection, Page, Session } from './data'
import type { RuntimeInfo, MCPInfo, CallbackRecord } from './backend'

export type Modal = 'connection' | 'session' | 'import' | 'commands' | null
export type DesignContext = {
  coreEpoch: number
  coreReady: boolean
  connections: Connection[]
  runtime: RuntimeInfo | null
  mcp: MCPInfo | null
  messages: CallbackRecord[]
  refresh: () => Promise<void>
  editConnection: (id: string) => void
  deleteConnection: (id: string) => Promise<void>
  font: string
  fontSize: string
  setFont: (value: string) => void
  setFontSize: (value: string) => void
  page: Page
  navigate: (page: Page) => void
  selected: Connection
  sessions: Session[]
  selectedSession: Session
  openSession: (id: string) => void
  inspectSession: (id: string) => void
  inspectedSessionId: string
  historyScope: { sessionId?: string; eventId?: string }
  inspectHistory: (sessionId: string, eventId?: string) => void
  endSessions: (ids: string[]) => Promise<void>
  removeSessions: (ids: string[]) => Promise<void>
  renameSession: (id: string, name: string) => Promise<void>
  setSessionManual: (id: string, manual: boolean) => Promise<void>
  openConnection: (id: string) => void
  tabs: string[]
  closeTab: (id: string) => void
  manual: boolean
  setManual: (manual: boolean) => void
  split: boolean
  setSplit: (split: boolean) => void
  openModal: (modal: Modal) => void
  notify: (text: string) => void
  theme: 'light' | 'dark'
  setTheme: (theme: 'light' | 'dark') => void
}

export const Design = createContext<DesignContext | null>(null)
export function useDesign() {
  const context = useContext(Design)
  if (!context) throw new Error('Shellink design context is missing')
  return context
}
