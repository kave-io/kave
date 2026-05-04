<script setup lang="ts">
import { ref, computed } from 'vue'
import { TRACES, RUNS, mockSpans, fmtMs, fmtMoney, type Span } from '@/data/mock'
import { KBtn, KCard, KStatusDot, KCopyBtn, KEmptyState, KKv } from '@/components/kv'

const currency = 'USD'

const traceId = ref(TRACES[0]!.id)
const trace = computed(() => TRACES.find(t => t.id === traceId.value)!)
const run = computed(() => (RUNS.find(r => r.traceId === traceId.value) || RUNS[0])!)
const spans = computed(() => mockSpans(run.value))
const totalDur = computed(() => run.value.duration || 4200)
const selSpan = ref<Span | null>(spans.value[1] ?? null)

function selectTrace(id: string) {
  traceId.value = id
  selSpan.value = null
}
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px; height: 100%;">
    <div class="page-h">
      <div>
        <h1>Traces</h1>
        <p>Waterfall view of every span. Find the slow span, the expensive call, the failing tool.</p>
      </div>
    </div>

    <div class="pane3" style="grid-template-columns: 280px minmax(0,1fr) 340px;">
      <div class="card pane3-rail" style="overflow: auto;">
        <div class="filter-bar"><div class="sh">Recent traces</div></div>
        <div>
          <button
            v-for="t in TRACES"
            :key="t.id"
            :style="{
              display: 'block', width: '100%', textAlign: 'left', padding: '10px 14px', border: 0,
              background: traceId === t.id ? 'var(--secondary-tint)' : 'transparent',
              borderBottom: '1px solid var(--border-soft)', cursor: 'pointer',
              borderLeft: traceId === t.id ? '2px solid var(--secondary)' : '2px solid transparent',
              fontFamily: 'inherit',
            }"
            @click="selectTrace(t.id)"
          >
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px;">
              <span class="mono" style="font-size: 12px;">{{ t.id.slice(0, 14) }}…</span>
              <KStatusDot :status="t.status" />
            </div>
            <div style="font-size: 12px; color: var(--text-muted); margin-top: 4px;">{{ t.agent }} · {{ t.spans }} spans</div>
            <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-faint); margin-top: 3px; font-family: var(--font-mono);">
              <span>{{ fmtMs(t.duration) }}</span>
              <span>{{ fmtMoney(t.spend, currency) }}</span>
            </div>
          </button>
        </div>
      </div>

      <div class="card" style="overflow: hidden; display: flex; flex-direction: column;">
        <div class="card-h">
          <div>
            <h3>Waterfall</h3>
            <div class="sub mono" style="font-size: 12px;">{{ trace.id }} · {{ fmtMs(trace.duration) }} total</div>
          </div>
          <div style="display: flex; gap: 6px;">
            <KCopyBtn :value="trace.id" label="trace id" />
            <KBtn size="sm" variant="ghost" icon="share">Export</KBtn>
          </div>
        </div>
        <div style="flex: 1; overflow: auto;">
          <div style="padding: 4px 0;">
            <div class="wf-row" style="background: var(--surface-2); cursor: default; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">
              <span>Span</span><span>Timeline</span><span class="num">Duration</span><span class="num">Tokens</span><span class="num">Cost</span>
            </div>
            <div
              v-for="s in spans"
              :key="s.id"
              :class="['wf-row', selSpan?.id === s.id ? 'selected' : '']"
              @click="selSpan = s"
            >
              <div class="wf-name" :style="{ paddingLeft: (s.depth * 14) + 'px' }">
                <span v-if="s.depth > 0" style="color: var(--text-faint);">└</span>
                <KStatusDot :status="s.status === 'ok' ? 'completed' : 'error'" />
                <span class="label">{{ s.name }}</span>
              </div>
              <div class="wf-track">
                <div :class="['wf-bar', s.kind]" :style="{ left: (s.start / totalDur * 100) + '%', width: Math.max(s.dur / totalDur * 100, 1) + '%' }" />
              </div>
              <span class="num mono" style="font-size: 11px;">{{ fmtMs(s.dur) }}</span>
              <span class="num mono" style="font-size: 11px; color: var(--text-dim);">{{ s.inputTokens ? `${s.inputTokens}→${s.outputTokens || 0}` : '—' }}</span>
              <span class="num mono" style="font-size: 11px;">{{ s.cost != null ? fmtMoney(s.cost, currency) : '—' }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="card pane3-detail" style="overflow: auto;">
        <div class="card-h">
          <div><h3>Span detail</h3><div class="sub">{{ selSpan?.name || '—' }}</div></div>
        </div>
        <div style="padding: 14px;">
          <KEmptyState v-if="!selSpan" icon="waypoints" title="Pick a span">
            Click any span in the waterfall to see identity, timing, payload, and cost.
          </KEmptyState>
          <div v-else>
            <KKv k="span id" mono>{{ selSpan.id }}</KKv>
            <KKv k="parent" mono>{{ selSpan.parent || '—' }}</KKv>
            <KKv k="kind">{{ selSpan.kind }}</KKv>
            <KKv k="status">{{ selSpan.status }}</KKv>
            <KKv k="start" mono>+{{ selSpan.start }}ms</KKv>
            <KKv k="duration" mono>{{ fmtMs(selSpan.dur) }}</KKv>
            <KKv v-if="selSpan.model" k="model" mono>{{ selSpan.provider }}/{{ selSpan.model }}</KKv>
            <KKv v-if="selSpan.method" k="method" mono>{{ selSpan.method }}</KKv>
            <KKv v-if="selSpan.inputTokens != null" k="tokens" mono>{{ selSpan.inputTokens }} → {{ selSpan.outputTokens || 0 }}</KKv>
            <KKv v-if="selSpan.cost != null" k="cost" mono>{{ fmtMoney(selSpan.cost, currency) }}</KKv>
            <div v-if="selSpan.error" style="margin-top: 12px; padding: 10px; background: var(--danger-tint); border: 1px solid rgba(179,38,30,0.25); border-radius: 6px; color: var(--danger); font-family: var(--font-mono); font-size: 12px;">
              {{ selSpan.error }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
