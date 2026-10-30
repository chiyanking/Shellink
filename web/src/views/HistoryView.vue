<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NSpace, NSpin, NSwitch } from 'naive-ui'
import { api } from '../api'
import { useI18n } from 'vue-i18n'
import { ansiToHtml } from '../utils/ansi'

const route = useRoute()
const router = useRouter()
const sessionId = route.params.id as string
const { t } = useI18n()

const text = ref('')
const loading = ref(false)
const colored = ref(true)
let cursor = 0

// The daemon keeps whatever the remote shell painted, but only hands it over
// when asked — so the request carries the toggle and the view simply renders
// what came back. With colour off the server has already stripped it, so the
// markup here is plain text either way.
const html = computed(() =>
  colored.value ? ansiToHtml(text.value) : escapeText(text.value),
)

function escapeText(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

async function load(reset = false) {
  loading.value = true
  try {
    if (reset) {
      cursor = 0
      text.value = ''
    }
    for (;;) {
      const res = await api<{ cursor: number; text: string }>(
        `/sessions/${sessionId}/history?since=${cursor}&limit=5000&color=${colored.value ? 1 : 0}`,
      )
      text.value += res.text
      if (res.cursor === cursor) break
      cursor = res.cursor
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => load())
</script>

<template>
  <div>
    <div class="page-header">
      <div style="display: flex; align-items: center; gap: 12px">
        <NButton size="small" quaternary @click="router.back()">
          ← {{ t('common.back') }}
        </NButton>
        <h1>{{ t('history.title') }} · {{ sessionId }}</h1>
      </div>
      <NSpace :size="10" align="center">
        <span style="font-size: 12px; opacity: 0.7">{{ t('history.color') }}</span>
        <NSwitch v-model:value="colored" size="small" @update:value="load(true)" />
        <NButton size="small" :loading="loading" @click="load(true)">{{ t('common.refresh') }}</NButton>
      </NSpace>
    </div>
    <NSpin :show="loading">
      <pre class="history-block" v-html="text ? html : t('history.empty')"></pre>
    </NSpin>
  </div>
</template>
