<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NButton,
  NCode,
  NEmpty,
  NModal,
  NPopconfirm,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
} from 'naive-ui'
import { api } from '../api'
import { useI18n } from 'vue-i18n'

interface McpToolProperty {
  type?: string
  description?: string
}
interface McpTool {
  name: string
  description?: string
  inputSchema?: {
    properties?: Record<string, McpToolProperty>
    required?: string[]
  }
}
interface McpInfo {
  enabled: boolean
  running: boolean
  rpcConnected: boolean
  transport: string
  host: string
  bindHost: string
  port: number
  path: string
  url: string
  token: string
  tokenSet: boolean
  command: string
  args: string[]
  clientConfig: Record<string, unknown>
  clientConfigStdio: Record<string, unknown>
  tools: McpTool[]
}

const { t } = useI18n()
const message = useMessage()
const info = ref<McpInfo | null>(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const showToken = ref(false)
const showQuickConfig = ref(false)

const httpConfigJson = computed(() => JSON.stringify(info.value?.clientConfig ?? {}, null, 2))
const stdioConfigJson = computed(() => JSON.stringify(info.value?.clientConfigStdio ?? {}, null, 2))
const maskedToken = computed(() => {
  const token = info.value?.token ?? ''
  if (!token) return ''
  if (showToken.value) return token
  return '•'.repeat(Math.min(token.length, 32))
})

function properties(tool: McpTool): Array<[string, McpToolProperty]> {
  return Object.entries(tool.inputSchema?.properties ?? {})
}
function isRequired(tool: McpTool, name: string): boolean {
  return (tool.inputSchema?.required ?? []).includes(name)
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    info.value = await api<McpInfo>('/mcp/info')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function act(path: string, body: unknown, okKey: string) {
  busy.value = true
  try {
    // NOTE: api() already JSON-encodes the body. Do NOT pre-stringify here,
    // otherwise the body is encoded twice and the server receives a string
    // instead of an object (e.g. mcp/config/enable and token regeneration
    // would silently no-op).
    await api(path, { method: 'POST', body: body ?? {} })
    message.success(t(okKey))
    await refresh()
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function toggleEnabled(next: boolean) {
  await act('/mcp/config', { enabled: next }, next ? 'mcp.enabledMsg' : 'mcp.disabledMsg')
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success(t('mcp.copied'))
  } catch {
    message.error(t('mcp.copyFailed'))
  }
}

async function stopService() {
  busy.value = true
  try {
    await api('/server/stop', { method: 'POST', body: '{}' })
    message.success(t('mcp.stopServiceRequested'))
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="mcp-page">
    <div class="page-header">
      <div>
        <h1>{{ t('mcp.title') }}</h1>
        <div class="sub">{{ t('app.mcpSub') }}</div>
      </div>
      <NSpace align="center">
        <NTag v-if="info?.running" size="small" type="success" :bordered="false">{{ t('mcp.running') }}</NTag>
        <NTag v-else size="small" :bordered="false">{{ t('mcp.stopped') }}</NTag>
        <NTag v-if="info?.rpcConnected" size="small" type="info" :bordered="false">{{ t('mcp.rpcConnected') }}</NTag>
        <NButton size="small" circle :loading="loading" @click="refresh">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12a9 9 0 1 1-2.6-6.4" />
            <path d="M21 3v6h-6" />
          </svg>
        </NButton>
      </NSpace>
    </div>

    <p class="hint">{{ t('mcp.hint') }}</p>

    <div class="row">
      <div class="row-label"><span>{{ t('mcp.enable') }}</span></div>
      <div class="row-value">
        <NSwitch :value="!!info?.enabled" :disabled="!info || busy" @update:value="toggleEnabled" />
      </div>
    </div>

    <div class="row">
      <div class="row-label"><span>{{ t('mcp.listenAddress') }}</span></div>
      <div class="row-value">
        <code class="pill">{{ info?.host ?? '-' }}</code>
        <code class="pill">{{ info?.port ?? '-' }}</code>
        <span v-if="info && info.bindHost !== info.host" class="muted">
          ({{ t('mcp.listenAddress') }} {{ info.bindHost }})
        </span>
      </div>
    </div>

    <div class="row">
      <div class="row-label"><span>{{ t('mcp.accessToken') }}</span></div>
      <div class="row-value">
        <code class="token">{{ maskedToken || '-' }}</code>
        <NButton size="tiny" quaternary @click="showToken = !showToken">
          {{ showToken ? t('mcp.hide') : t('mcp.reveal') }}
        </NButton>
        <NButton
          size="tiny"
          :loading="busy"
          @click="act('/mcp/token', { regenerateToken: true }, 'mcp.tokenRegenerated')"
        >
          {{ t('mcp.regenerate') }}
        </NButton>
      </div>
    </div>

    <div class="action-bar">
      <NSpace :size="8" align="center">
        <NButton size="small" :disabled="!info || info.enabled || busy" @click="act('/mcp/start', {}, 'mcp.started')">
          {{ t('mcp.start') }}
        </NButton>
        <NButton size="small" :disabled="!info || busy" @click="act('/mcp/restart', {}, 'mcp.restarted')">
          {{ t('mcp.restart') }}
        </NButton>
        <NButton size="small" type="error" :disabled="!info || !info.enabled || busy" @click="act('/mcp/stop', {}, 'mcp.stoppedMsg')">
          {{ t('mcp.stop') }}
        </NButton>
      </NSpace>
      <code class="url">{{ info?.url ?? '-' }}</code>
      <NButton size="small" :disabled="!info?.url" @click="copy(info?.url ?? '')">{{ t('mcp.copy') }}</NButton>
      <NButton size="small" type="primary" :disabled="!info" @click="showQuickConfig = true">
        {{ t('mcp.quickConfig') }}
      </NButton>
    </div>

    <section class="block">
      <header class="block-header">
        <h2>{{ t('mcp.tools') }}</h2>
        <NTag v-if="info" size="small" :bordered="false">{{ info.tools.length }}</NTag>
      </header>
      <NEmpty v-if="!info && !loading" :description="t('mcp.empty')" class="empty" />
      <div v-else class="tool-list">
        <article v-for="tool in info?.tools ?? []" :key="tool.name" class="tool-item">
          <div class="tool-name">{{ tool.name }}</div>
          <div v-if="tool.description" class="tool-desc">{{ tool.description }}</div>
          <div v-if="properties(tool).length > 0" class="tool-params">
            <span class="params-label">{{ t('mcp.params') }}:</span>
            <span
              v-for="[name, prop] in properties(tool)"
              :key="name"
              class="param"
              :class="{ required: isRequired(tool, name) }"
            >
              {{ name }}<em v-if="prop?.type">: {{ prop.type }}</em>
            </span>
          </div>
        </article>
      </div>
    </section>

    <section class="block danger">
      <header class="block-header">
        <h2>{{ t('mcp.stopService') }}</h2>
      </header>
      <NPopconfirm @positive-click="stopService">
        <template #trigger>
          <NButton size="small" type="error" :loading="busy">{{ t('mcp.stopService') }}</NButton>
        </template>
        {{ t('mcp.stopServiceConfirm') }}
      </NPopconfirm>
    </section>

    <p v-if="error" class="error">{{ error }}</p>

    <NModal
      v-model:show="showQuickConfig"
      preset="card"
      :title="t('mcp.quickConfigTitle')"
      style="width: 720px"
    >
      <h3 class="modal-sub">{{ t('mcp.httpMode') }}</h3>
      <NCode :code="httpConfigJson" language="json" word-wrap />
      <div class="modal-actions">
        <NButton size="small" @click="copy(httpConfigJson)">{{ t('mcp.copyConfig') }}</NButton>
      </div>
      <h3 class="modal-sub">{{ t('mcp.stdioMode') }}</h3>
      <NCode :code="stdioConfigJson" language="json" word-wrap />
      <div class="modal-actions">
        <NButton size="small" @click="copy(stdioConfigJson)">{{ t('mcp.copyConfig') }}</NButton>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.mcp-page {
  max-width: 1120px;
  margin: 0 auto;
}
.hint {
  margin: 0 0 16px;
  color: var(--text-muted);
  font-size: 13px;
  line-height: 1.7;
}
.row {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface);
  margin-bottom: 10px;
}
.row-label {
  color: var(--text-soft);
  font-size: 13px;
}
.row-value {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}
.pill {
  padding: 3px 10px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--surface-muted);
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--text);
}
.token {
  min-width: 0;
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--text);
}
.muted {
  color: var(--text-muted);
  font-size: 12px;
}
.action-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 12px 14px;
  margin: 16px 0 24px;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-muted);
}
.action-bar .url {
  flex: 1 1 260px;
  min-width: 0;
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--text);
}
.block {
  margin-bottom: 24px;
}
.block.danger {
  padding: 14px 16px;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface);
}
.block-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.block-header h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.block :deep(pre) {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.tool-list {
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface);
  overflow: hidden;
}
.tool-item {
  padding: 13px 16px;
  border-bottom: 1px solid var(--line);
}
.tool-item:last-child {
  border-bottom: 0;
}
.tool-name {
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.tool-desc {
  margin-top: 4px;
  color: var(--text-muted);
  font-size: 12.5px;
  line-height: 1.6;
}
.tool-params {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}
.params-label {
  color: var(--text-soft);
}
.param {
  padding: 2px 7px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface-muted);
  color: var(--text-muted);
  font-family: var(--font-mono);
}
.param.required {
  border-color: var(--accent, #4c8dff);
  color: var(--accent, #4c8dff);
}
.param em {
  font-style: normal;
  opacity: 0.7;
}
.empty {
  margin: 48px 0;
}
.error {
  color: var(--error, #e06c75);
  font-size: 12.5px;
}
.modal-sub {
  margin: 14px 0 8px;
  font-size: 13px;
  font-weight: 600;
}
.modal-actions {
  margin-top: 8px;
}
@media (max-width: 640px) {
  .row {
    grid-template-columns: 1fr;
  }
}
</style>
