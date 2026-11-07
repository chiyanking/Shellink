import { createContext, useContext } from 'react'
import type { Connection, Page, Session } from './data'
import type { RuntimeInfo, MCPInfo } from './backend'

export type Modal = 'connection' | 'session' | 'import' | 'commands' | null
export type SettingsTab = 'general' | 'appearance' | 'advanced'
export type AIClient = 'Claude Code' | 'Codex' | 'Cursor'
export type DesignContext = {
  coreEpoch: number
  coreReady: boolean
  connections: Connection[]
  connectionGroup: string | null
  setConnectionGroup: (group: string | null) => void
  connectionGroups: string[]
  addConnectionGroup: (name: string) => string | null
  deleteConnectionGroup: (name: string) => boolean
  moveConnectionToGroup: (id: string, group: string) => void
  draggedConnectionId: string | null
  setDraggedConnectionId: (id: string | null) => void
  settingsTab: SettingsTab
  setSettingsTab: (tab: SettingsTab) => void
  client: AIClient
  setClient: (client: AIClient) => void
  runtime: RuntimeInfo | null
  mcp: MCPInfo | null
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
  historyScope: { sessionId?: string; eventId?: string }
  inspectHistory: (sessionId: string, eventId?: string) => void
  setSessionManual: (id: string, manual: boolean) => Promise<void>
  openConnection: (id: string) => void
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
