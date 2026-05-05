<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { eventsClient } from '@/lib/rpc/clients'
import { envId, projectId } from '@/stores/workspace'
import type { LiveEvent } from '@/types/api'
import { fmtMoney, fmtMs, fmtTime } from '@/lib/format'
import KBadge from './KBadge.vue'
import KEmptyState from './KEmptyState.vue'

const props = withDefaults(defineProps<{
  limit?: number
  compact?: boolean
  paused?: boolean
  filters?: { agent?: string; provider?: string; errorsOnly?: boolean; blockedOnly?: boolean }
  currency?: string
}>(), {
  limit: 50,
  compact: false,
  paused: false,
  currency: 'USD',
  filters: () => ({}),
})

const emit = defineEmits<{ (e: 'pick', ev: LiveEvent): void }>()

const events = ref<LiveEvent[]>([])
const isLive = ref(false)
const error = ref<string | null>(null)
let controller: AbortController | null = null

async function start() {
  if (props.paused || controller) return
  controller = new AbortController()
  error.value = null
  try {
    const stream = await eventsClient.watch(
      { projectId: projectId.value, envId: envId.value },
      controller.signal,
    )
    isLive.value = true
    for await (const ev of stream) {
      events.value = [ev, ...events.value].slice(0, 200)
    }
  } catch (err) {
    if (!controller?.signal.aborted) {
      error.value = err instanceof Error ? err.message : 'Live stream disconnected'
    }
  } finally {
    isLive.value = false
    controller = null
  }
}

function stop() {
  controller?.abort()
  controller = null
  isLive.value = false
}

onMounted(start)
onBeforeUnmount(stop)

watch(() => props.paused, (paused) => {
  if (paused) stop()
  else void start()
})

const filtered = computed<LiveEvent[]>(() => {
  return events.value.filter(ev => {
    const f = props.filters || {}
    if (f.agent && ev.agent !== f.agent && ev.agentId !== f.agent) return false
    if (f.provider && ev.provider !== f.provider) return false
    if (f.errorsOnly && ev.tone !== 'danger') return false
    if (f.blockedOnly && !ev.kind.includes('blocked') && !ev.kind.includes('denied')) return false
    return true
  }).slice(0, props.limit)
})
</script>

<template>
  <div>
    <div v-if="error" style="padding: 10px 12px; color: var(--danger); font-size: 12px; border-bottom: 1px solid var(--border-soft);">
      {{ error }}
    </div>
    <div
      v-for="(ev, i) in filtered"
      :key="ev.id"
      :class="['evrow', i === 0 && !paused ? 'new' : '']"
      @click="emit('pick', ev)"
    >
      <span class="ts">{{ fmtTime(ev.ts) }}</span>
      <KBadge :tone="ev.tone">{{ ev.kind }}</KBadge>
      <div class="meta">
        <span class="agent">{{ ev.agent }}</span>
        <span style="color: var(--text-faint);">.</span>
        <span class="model">{{ ev.provider || 'daemon' }}/{{ ev.model || ev.method }}</span>
      </div>
      <div class="right">
        <span v-if="!compact && ev.duration != null" class="mono" :style="{ color: 'var(--text-dim)', fontSize: '12px', minWidth: '50px', textAlign: 'right' }">{{ fmtMs(ev.duration) }}</span>
        <span v-if="ev.cost != null" class="mono" :style="{ minWidth: '60px', textAlign: 'right', fontSize: '12px' }">{{ fmtMoney(ev.cost, currency) }}</span>
        <span v-else class="mono" :style="{ minWidth: '60px', textAlign: 'right', fontSize: '12px', color: 'var(--text-faint)' }">-</span>
      </div>
    </div>
    <KEmptyState v-if="filtered.length === 0" icon="activity" :title="isLive ? 'Waiting for live traffic' : 'No live events yet'">
      Point an SDK at Kave and run an agent. Runtime events will appear here as the daemon emits them.
    </KEmptyState>
  </div>
</template>
