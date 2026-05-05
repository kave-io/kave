<script setup lang="ts">
import { computed, ref, watchEffect } from 'vue'
import { useRuns } from '@/composables/api/useRuns'
import { useTraceGraph } from '@/composables/api/useTraceGraph'
import { envId, projectId } from '@/stores/workspace'
import type { Span } from '@/types/api'
import { asNumber, fmtMoney, fmtMs, fmtRel } from '@/lib/format'
import { KBtn, KCard, KStatusDot, KCopyBtn, KEmptyState, KKv } from '@/components/kv'

const currency = 'USD'
const runsQuery = useRuns({ projectId, envId, limit: 200 })
const traceId = ref('')
const runs = computed(() => runsQuery.data.value ?? [])
const run = computed(() => runs.value.find(r => r.id === traceId.value) ?? runs.value[0] ?? null)
const graphQuery = useTraceGraph(computed(() => run.value?.id ?? ''), 1000)
const spans = computed(() => graphQuery.data.value?.spans ?? [])
const totalDur = computed(() => graphQuery.data.value?.total_duration_ms || (run.value?.ended_at ? run.value.ended_at - run.value.started_at : Math.max(...spans.value.map(s => s.duration_ms), 1)))
const selSpan = ref<Span | null>(null)

watchEffect(() => {
  if (!traceId.value && runs.value[0]) traceId.value = runs.value[0].id
  if (run.value && !selSpan.value && spans.value[0]) selSpan.value = spans.value[0]
})

function selectTrace(id: string) {
  traceId.value = id
  selSpan.value = null
}

function spanStart(span: Span) {
  const base = run.value?.started_at ?? spans.value[0]?.started_at ?? span.started_at
  return Math.max(0, span.started_at - base)
}
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px; height: 100%;">
    <div class="page-h"><div><h1>Traces</h1><p>Waterfall view of daemon spans. Find the slow span, the expensive call, the failing tool.</p></div></div>

    <div class="pane3" style="grid-template-columns: 280px minmax(0,1fr) 340px;">
      <div class="card pane3-rail" style="overflow: auto;">
        <div class="filter-bar"><div class="sh">Recent traces</div></div>
        <button v-for="r in runs" :key="r.id" :style="{ display: 'block', width: '100%', textAlign: 'left', padding: '10px 14px', border: 0, background: traceId === r.id ? 'var(--secondary-tint)' : 'transparent', borderBottom: '1px solid var(--border-soft)', cursor: 'pointer', borderLeft: traceId === r.id ? '2px solid var(--secondary)' : '2px solid transparent', fontFamily: 'inherit' }" @click="selectTrace(r.id)">
          <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px;"><span class="mono" style="font-size: 12px;">{{ r.id.slice(0, 18) }}</span><KStatusDot :status="r.status" /></div>
          <div style="font-size: 12px; color: var(--text-muted); margin-top: 4px;">{{ r.agent_id || 'unknown-agent' }}</div>
          <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-faint); margin-top: 3px; font-family: var(--font-mono);"><span>{{ fmtRel(r.started_at) }}</span><span>{{ fmtMoney(r.spent, currency) }}</span></div>
        </button>
        <KEmptyState v-if="runs.length === 0" icon="waypoints" title="No traces yet">Run traffic through Kave to populate traces and spans.</KEmptyState>
      </div>

      <div class="card" style="overflow: hidden; display: flex; flex-direction: column;">
        <div class="card-h"><div><h3>Waterfall</h3><div class="sub mono" style="font-size: 12px;">{{ run?.id || '-' }} · {{ fmtMs(totalDur) }} total</div></div><div style="display: flex; gap: 6px;"><KCopyBtn v-if="run" :value="run.id" label="trace id" /><KBtn size="sm" variant="ghost" icon="share">Export</KBtn></div></div>
        <div style="flex: 1; overflow: auto;">
          <div style="padding: 4px 0;">
            <div class="wf-row" style="background: var(--surface-2); cursor: default; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);"><span>Span</span><span>Timeline</span><span class="num">Duration</span><span class="num">Tokens</span><span class="num">Cost</span></div>
            <div v-for="s in spans" :key="s.id" :class="['wf-row', selSpan?.id === s.id ? 'selected' : '']" @click="selSpan = s">
              <div class="wf-name" :style="{ paddingLeft: (s.parent_id ? 14 : 0) + 'px' }"><span v-if="s.parent_id" style="color: var(--text-faint);">└</span><KStatusDot :status="s.error ? 'error' : 'completed'" /><span class="label">{{ s.name }}</span></div>
              <div class="wf-track"><div :class="['wf-bar', s.error ? 'error' : s.model ? 'llm' : 'tool']" :style="{ left: (spanStart(s) / totalDur * 100) + '%', width: Math.max(s.duration_ms / totalDur * 100, 1) + '%' }" /></div>
              <span class="num mono" style="font-size: 11px;">{{ fmtMs(s.duration_ms) }}</span>
              <span class="num mono" style="font-size: 11px; color: var(--text-dim);">{{ s.input_tokens ? `${s.input_tokens}->${s.output_tokens || 0}` : '-' }}</span>
              <span class="num mono" style="font-size: 11px;">{{ s.cost ? fmtMoney(s.cost, currency) : '-' }}</span>
            </div>
            <KEmptyState v-if="spans.length === 0" icon="activity" title="No spans for this run">Span records will appear here when tracing captures actions.</KEmptyState>
          </div>
        </div>
      </div>

      <div class="card pane3-detail" style="overflow: auto;">
        <div class="card-h"><div><h3>Span detail</h3><div class="sub">{{ selSpan?.name || '-' }}</div></div></div>
        <div style="padding: 14px;">
          <KEmptyState v-if="!selSpan" icon="waypoints" title="Pick a span">Click any span in the waterfall to see identity, timing, payload, and cost.</KEmptyState>
          <div v-else>
            <KKv k="span id" mono>{{ selSpan.id }}</KKv><KKv k="parent" mono>{{ selSpan.parent_id || '-' }}</KKv><KKv k="status">{{ selSpan.error ? 'error' : 'ok' }}</KKv><KKv k="started" mono>{{ new Date(selSpan.started_at).toISOString() }}</KKv><KKv k="duration" mono>{{ fmtMs(selSpan.duration_ms) }}</KKv><KKv v-if="selSpan.model" k="model" mono>{{ selSpan.model }}</KKv><KKv v-if="selSpan.input_tokens != null" k="tokens" mono>{{ selSpan.input_tokens }} -> {{ selSpan.output_tokens || 0 }}</KKv><KKv v-if="selSpan.cost" k="cost" mono>{{ fmtMoney(asNumber(selSpan.cost), currency) }}</KKv>
            <div v-if="selSpan.error" style="margin-top: 12px; padding: 10px; background: var(--danger-tint); border: 1px solid rgba(179,38,30,0.25); border-radius: 6px; color: var(--danger); font-family: var(--font-mono); font-size: 12px;">{{ selSpan.error }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
