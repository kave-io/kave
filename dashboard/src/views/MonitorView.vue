<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAgents } from '@/composables/api/useAgents'
import { envId } from '@/stores/workspace'
import { fmtMs, fmtMoney } from '@/lib/format'
import type { LiveEvent } from '@/types/api'
import { KIcon, KBtn, KBadge, KCard, KEmptyState, KLiveStream, KKv, KCopyBtn } from '@/components/kv'

const router = useRouter()
const agentsQuery = useAgents(envId)

const paused = ref(false)
const filters = ref<{ agent: string; provider: string; errorsOnly: boolean; blockedOnly: boolean }>({ agent: '', provider: '', errorsOnly: false, blockedOnly: false })
const selected = ref<LiveEvent | null>(null)

const providerNames = ['openai', 'github']
const agentOpts = computed(() => [{ v: '', l: 'All' }, ...(agentsQuery.data.value ?? []).map(a => ({ v: a.id, l: a.name }))])
const providerOpts = computed(() => [{ v: '', l: 'All' }, ...providerNames.map(p => ({ v: p, l: p }))])
const kindOpts = [{ v: '', l: 'All' }, { v: 'errorsOnly', l: 'Errors only' }, { v: 'blockedOnly', l: 'Blocked only' }]
const kindValue = computed(() => filters.value.errorsOnly ? 'errorsOnly' : filters.value.blockedOnly ? 'blockedOnly' : '')

function setKind(v: string) {
  filters.value.errorsOnly = v === 'errorsOnly'
  filters.value.blockedOnly = v === 'blockedOnly'
}

const rawPayload = computed(() => selected.value ? JSON.stringify({
  event: selected.value.kind,
  ts: selected.value.ts,
  run_id: selected.value.runId,
  trace_id: selected.value.traceId,
  agent: { id: selected.value.agentId, name: selected.value.agent },
  provider: selected.value.provider,
  model: selected.value.model,
  method: selected.value.method,
  metrics: { duration_ms: selected.value.duration, cost_usd: selected.value.cost, input_tokens: selected.value.inputTokens, output_tokens: selected.value.outputTokens },
}, null, 2) : '')
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px; height: 100%;">
    <div class="page-h">
      <div>
        <h1>Live monitor</h1>
        <p>Streaming runs, spans, policy decisions, and cost events from the local daemon.</p>
      </div>
      <div class="toolbar">
        <KBadge :tone="paused ? 'neutral' : 'success'" :dot="paused ? null : 'live'">{{ paused ? 'Paused' : 'Live · 18080' }}</KBadge>
        <KBtn :icon="paused ? 'play' : 'pause'" size="sm" @click="paused = !paused">{{ paused ? 'Resume' : 'Pause' }}</KBtn>
        <KBtn icon="filter" size="sm">Export NDJSON</KBtn>
      </div>
    </div>

    <section class="pane3">
      <div class="card pane3-rail" style="overflow: auto;">
        <div style="padding: 12px 14px; border-bottom: 1px solid var(--border-soft);">
          <div class="sh">Filters</div>
        </div>
        <div style="padding: 14px; display: flex; flex-direction: column; gap: 14px;">
          <div>
            <div class="sh" style="margin-bottom: 6px;">Agent</div>
            <div style="display: flex; flex-direction: column; gap: 2px;">
              <label
                v-for="o in agentOpts"
                :key="o.v"
                :style="{ display: 'flex', alignItems: 'center', gap: '8px', padding: '5px 6px', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', color: filters.agent === o.v ? 'var(--text)' : 'var(--text-muted)', background: filters.agent === o.v ? 'var(--secondary-tint)' : 'transparent' }"
              >
                <input type="radio" name="agent" :checked="filters.agent === o.v" :style="{ accentColor: 'var(--secondary)' }" @change="filters.agent = o.v" />
                <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ o.l }}</span>
              </label>
            </div>
          </div>
          <div>
            <div class="sh" style="margin-bottom: 6px;">Provider</div>
            <div style="display: flex; flex-direction: column; gap: 2px;">
              <label
                v-for="o in providerOpts"
                :key="o.v"
                :style="{ display: 'flex', alignItems: 'center', gap: '8px', padding: '5px 6px', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', color: filters.provider === o.v ? 'var(--text)' : 'var(--text-muted)', background: filters.provider === o.v ? 'var(--secondary-tint)' : 'transparent' }"
              >
                <input type="radio" name="provider" :checked="filters.provider === o.v" :style="{ accentColor: 'var(--secondary)' }" @change="filters.provider = o.v" />
                <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ o.l }}</span>
              </label>
            </div>
          </div>
          <div>
            <div class="sh" style="margin-bottom: 6px;">Event kind</div>
            <div style="display: flex; flex-direction: column; gap: 2px;">
              <label
                v-for="o in kindOpts"
                :key="o.v"
                :style="{ display: 'flex', alignItems: 'center', gap: '8px', padding: '5px 6px', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', color: kindValue === o.v ? 'var(--text)' : 'var(--text-muted)', background: kindValue === o.v ? 'var(--secondary-tint)' : 'transparent' }"
              >
                <input type="radio" name="kind" :checked="kindValue === o.v" :style="{ accentColor: 'var(--secondary)' }" @change="setKind(o.v)" />
                <span>{{ o.l }}</span>
              </label>
            </div>
          </div>
        </div>
      </div>

      <div class="card" style="overflow: auto; min-height: 0;">
        <KLiveStream :limit="50" :paused="paused" :filters="filters" @pick="selected = $event" />
      </div>

      <div class="card pane3-detail" style="overflow: auto; display: flex; flex-direction: column;">
        <header class="card-h">
          <div>
            <h3>Event detail</h3>
            <div class="sub">{{ selected ? selected.kind : 'Click any row to inspect' }}</div>
          </div>
          <KBadge v-if="selected" :tone="selected.tone">{{ selected.kind }}</KBadge>
        </header>
        <div style="padding: 14px; flex: 1; overflow: auto;">
          <KEmptyState v-if="!selected" icon="zap" title="Pick an event">
            Click any row in the live stream to see the full payload, related run, and policy decision.
          </KEmptyState>
          <div v-else>
            <KKv k="event">{{ selected.kind }}</KKv>
            <KKv k="time" mono>{{ new Date(selected.ts).toISOString() }}</KKv>
            <KKv k="agent">{{ selected.agent }}</KKv>
            <KKv k="run id" mono>
              {{ selected.runId }}
              <template #action><KCopyBtn :value="selected.runId" /></template>
            </KKv>
            <KKv k="trace id" mono>
              {{ selected.traceId }}
              <template #action><KCopyBtn :value="selected.traceId" /></template>
            </KKv>
            <KKv k="provider" mono>{{ selected.provider }} / {{ selected.model }}</KKv>
            <KKv k="method" mono>{{ selected.method }}</KKv>
            <KKv k="duration" mono>{{ fmtMs(selected.duration) }}</KKv>
            <KKv k="tokens" mono>{{ selected.inputTokens }} → {{ selected.outputTokens }}</KKv>
            <KKv k="cost" mono>{{ selected.cost != null ? fmtMoney(selected.cost) : 'n/a (denied)' }}</KKv>

            <div style="margin-top: 14px;">
              <div class="sh" style="margin-bottom: 6px;">Raw payload</div>
              <pre class="code" style="white-space: pre-wrap;">{{ rawPayload }}</pre>
            </div>
            <div style="display: flex; gap: 6px; margin-top: 12px;">
              <KBtn size="sm" icon="waypoints" @click="router.push('/traces')">Open trace</KBtn>
              <KBtn size="sm" variant="ghost" icon="file-text" @click="router.push('/runs')">Open run</KBtn>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
