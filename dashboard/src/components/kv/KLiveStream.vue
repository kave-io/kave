<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { SEED_EVENTS, genEvent, fmtTime, fmtMs, fmtMoney, type LiveEvent } from '@/data/mock'
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

const events = ref<LiveEvent[]>([...SEED_EVENTS])
let timer: ReturnType<typeof setInterval> | null = null

function start() {
  if (timer) clearInterval(timer)
  timer = setInterval(() => {
    events.value = [genEvent(), ...events.value].slice(0, 200)
  }, 1300 + Math.random() * 1800)
}
function stop() { if (timer) { clearInterval(timer); timer = null } }

onMounted(() => { if (!props.paused) start() })
onBeforeUnmount(stop)

watch(() => props.paused, (p) => { if (p) stop(); else start() })

const filtered = computed<LiveEvent[]>(() => {
  return events.value.filter(ev => {
    const f = props.filters || {}
    if (f.agent && ev.agent !== f.agent) return false
    if (f.provider && ev.provider !== f.provider) return false
    if (f.errorsOnly && ev.tone !== 'danger') return false
    if (f.blockedOnly && ev.kind !== 'policy.denied') return false
    return true
  }).slice(0, props.limit)
})
</script>

<template>
  <div>
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
        <span style="color: var(--text-faint);">·</span>
        <span class="model">{{ ev.provider }}/{{ ev.model }}</span>
      </div>
      <div class="right">
        <span v-if="!compact && ev.duration != null" class="mono" :style="{ color: 'var(--text-dim)', fontSize: '12px', minWidth: '50px', textAlign: 'right' }">{{ fmtMs(ev.duration) }}</span>
        <span v-if="ev.cost != null" class="mono" :style="{ minWidth: '60px', textAlign: 'right', fontSize: '12px' }">{{ fmtMoney(ev.cost, currency) }}</span>
        <span v-else class="mono" :style="{ minWidth: '60px', textAlign: 'right', fontSize: '12px', color: 'var(--text-faint)' }">—</span>
      </div>
    </div>
    <KEmptyState v-if="filtered.length === 0" icon="activity" title="No events match filters">
      Adjust filters or run an agent to produce traffic.
    </KEmptyState>
  </div>
</template>
