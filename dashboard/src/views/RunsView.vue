<script setup lang="ts">
import { ref, computed } from 'vue'
import { RUNS, mockSpans, fmtMs, fmtRel, fmtMoney, shortId, type Run, type RunStatus } from '@/data/mock'
import { KIcon, KBtn, KBadge, KCard, KStatusDot, KStatusBadge, KCopyBtn, KKv, KMini } from '@/components/kv'

const currency = 'USD'

const search = ref('')
const statusFilter = ref<RunStatus | 'all'>('all')
const selected = ref<Run | null>(null)
const tab = ref<'timeline' | 'spans' | 'cost' | 'policy' | 'i/o' | 'raw'>('timeline')

const filtered = computed(() => RUNS.filter(r => {
  if (statusFilter.value !== 'all' && r.status !== statusFilter.value) return false
  if (search.value && !(r.name + r.agent + r.id).toLowerCase().includes(search.value.toLowerCase())) return false
  return true
}))

const STATUSES: (RunStatus | 'all')[] = ['all', 'running', 'completed', 'failed', 'blocked']

const spans = computed(() => selected.value ? mockSpans(selected.value) : [])
const totalDur = computed(() => selected.value?.duration || 4200)

const llmSpans = computed(() => spans.value.filter(s => s.kind === 'llm'))
const totalCost = computed(() => llmSpans.value.reduce((a, s) => a + (s.cost || 0), 0))
const totalIn = computed(() => llmSpans.value.reduce((a, s) => a + (s.inputTokens || 0), 0))
const totalOut = computed(() => llmSpans.value.reduce((a, s) => a + (s.outputTokens || 0), 0))

const inputJson = computed(() => selected.value ? `{
  "messages": [
    { "role": "system", "content": "You are a helpful coding assistant." },
    { "role": "user", "content": "Summarize the open PRs in kave-io/kave and group them by area." }
  ],
  "model": "${selected.value.model}",
  "tools": [{ "type": "function", "function": { "name": "github_search" } }]
}` : '')

const outputJson = computed(() => {
  const r = selected.value; if (!r) return ''
  return r.status === 'failed'
    ? `{
  "error": {
    "type": "connection_error",
    "message": "upstream timeout after 30s"
  }
}`
    : `{
  "id": "chatcmpl-${shortId()}",
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "Open PRs in kave-io/kave (12 total):\\n\\n**Dashboard (5)**\\n- #421 Live monitor empty state\\n- #418 Trace waterfall keyboard nav\\n…"
    },
    "finish_reason": "stop"
  }],
  "usage": { "prompt_tokens": ${r.inputTokens}, "completion_tokens": ${r.outputTokens} }
}`
})
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div>
        <h1>Runs</h1>
        <p>Every agent execution. The unit of debugging in Kave.</p>
      </div>
      <div class="toolbar">
        <KBtn icon="filter" size="sm">Export</KBtn>
      </div>
    </div>

    <div class="card" style="overflow: hidden;">
      <div class="filter-bar">
        <div style="position: relative; flex: 0 1 280px;">
          <KIcon name="search" :size="14" :style="{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-faint)' }" />
          <input v-model="search" class="input mono" placeholder="run id / agent / model …" style="padding-left: 30px;" />
        </div>
        <div style="display: flex; gap: 4px;">
          <button
            v-for="s in STATUSES"
            :key="s"
            class="btn btn-sm"
            :style="{
              background: statusFilter === s ? 'var(--secondary-tint)' : 'transparent',
              color: statusFilter === s ? 'var(--text)' : 'var(--text-dim)',
              border: '1px solid ' + (statusFilter === s ? 'var(--secondary-edge)' : 'transparent'),
            }"
            @click="statusFilter = s"
          >{{ s }}</button>
        </div>
        <div style="margin-left: auto; font-size: 12px; color: var(--text-dim);">{{ filtered.length }} runs</div>
      </div>

      <div class="tbl-scroll">
        <table class="tbl">
          <thead>
            <tr>
              <th style="width: 32px"></th>
              <th>Run</th><th>Agent</th><th>Model</th><th>Started</th>
              <th class="num">Duration</th><th class="num">Spend</th><th class="num">Tokens</th><th class="num">Spans</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in filtered" :key="r.id" :class="selected?.id === r.id ? 'selected' : ''" @click="selected = r; tab = 'timeline'">
              <td><KStatusDot :status="r.status" /></td>
              <td>
                <div style="font-weight: 500;">{{ r.name }}</div>
                <div class="mono" style="font-size: 11px; color: var(--text-faint);">{{ r.id }}</div>
              </td>
              <td>{{ r.agent }}</td>
              <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ r.provider }}/{{ r.model }}</td>
              <td style="color: var(--text-muted); font-size: 12px;">{{ fmtRel(r.started) }}</td>
              <td class="num mono" style="font-size: 12px;">{{ fmtMs(r.duration) }}</td>
              <td class="num mono" style="font-size: 12px;">{{ fmtMoney(r.spend, currency) }}</td>
              <td class="num mono" style="font-size: 12px; color: var(--text-muted);">{{ (r.inputTokens / 1000).toFixed(1) }}k→{{ (r.outputTokens / 1000).toFixed(1) }}k</td>
              <td class="num" style="color: var(--text-muted);">{{ r.spans }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template v-if="selected">
      <div class="drawer-overlay" @click="selected = null" />
      <aside class="drawer" role="dialog" aria-label="Run detail">
        <header class="drawer-h">
          <KStatusBadge :status="selected.status" />
          <div style="flex: 1; min-width: 0;">
            <div class="title" style="display: flex; align-items: center; gap: 8px;">
              {{ selected.name }}
              <span class="mono" style="font-size: 12px; font-weight: 400; color: var(--text-faint);">{{ selected.id }}</span>
              <KCopyBtn :value="selected.id" />
            </div>
            <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px;">
              {{ selected.agent }} · {{ selected.policy }} · started {{ fmtRel(selected.started) }}
            </div>
          </div>
          <KBtn variant="ghost" icon="x" aria-label="Close" @click="selected = null" />
        </header>

        <div :style="{ padding: '12px 18px', display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '14px', borderBottom: '1px solid var(--border-soft)' }">
          <KMini label="Duration" :value="fmtMs(selected.duration)" />
          <KMini label="Spend" :value="fmtMoney(selected.spend, currency)" />
          <KMini label="Tokens" :value="`${(selected.inputTokens/1000).toFixed(1)}k → ${(selected.outputTokens/1000).toFixed(1)}k`" />
          <KMini label="Spans" :value="selected.spans" />
          <KMini label="Trace" :value="selected.traceId" mono small />
        </div>

        <div class="tabs">
          <button v-for="t in (['timeline','spans','cost','policy','i/o','raw'] as const)" :key="t" :class="['tab', tab === t ? 'active' : '']" @click="tab = t">{{ t }}</button>
        </div>

        <div class="drawer-b">
          <div v-if="tab === 'timeline'" style="padding: 16px;">
            <div v-for="s in spans" :key="s.id" class="wf-row" style="grid-template-columns: 24px 220px 1fr 90px 70px;">
              <span><KStatusDot :status="s.status === 'ok' ? 'completed' : 'error'" /></span>
              <div class="wf-name" :style="{ paddingLeft: (s.depth * 14) + 'px' }">
                <span class="label">{{ s.name }}</span>
              </div>
              <div class="wf-track">
                <div :class="['wf-bar', s.kind]" :style="{ left: (s.start / totalDur * 100) + '%', width: Math.max(s.dur / totalDur * 100, 1) + '%' }">
                  <span class="lab">{{ fmtMs(s.dur) }}</span>
                </div>
              </div>
              <span class="num mono" style="font-size: 11px; color: var(--text-dim);">{{ fmtMs(s.dur) }}</span>
              <span class="num mono" style="font-size: 11px;">{{ s.cost != null ? fmtMoney(s.cost) : '—' }}</span>
            </div>
          </div>

          <div v-else-if="tab === 'spans'" style="padding: 16px;">
            <div
              v-for="s in spans"
              :key="s.id"
              :style="{
                paddingLeft: (s.depth * 18) + 'px',
                padding: '8px 0',
                borderBottom: '1px solid var(--border-soft)',
                display: 'grid',
                gridTemplateColumns: '1fr auto',
                gap: '12px',
                alignItems: 'center',
              }"
            >
              <div :style="{ paddingLeft: (s.depth * 18) + 'px' }">
                <div style="display: flex; align-items: center; gap: 6px;">
                  <span v-if="s.depth > 0" style="color: var(--text-faint);">└</span>
                  <span class="mono" style="font-size: 12px;">{{ s.name }}</span>
                  <span v-if="s.model" class="mono" style="font-size: 11px; color: var(--text-dim);">{{ s.provider }}/{{ s.model }}</span>
                </div>
                <div v-if="s.error" style="font-size: 12px; color: var(--danger); margin-top: 4px;">{{ s.error }}</div>
              </div>
              <div style="display: flex; gap: 12px; font-size: 11px; font-family: var(--font-mono); color: var(--text-dim);">
                <span>{{ fmtMs(s.dur) }}</span>
                <span v-if="s.inputTokens">{{ s.inputTokens }}→{{ s.outputTokens || 0 }} tok</span>
                <span v-if="s.cost != null" style="color: var(--text);">{{ fmtMoney(s.cost) }}</span>
              </div>
            </div>
          </div>

          <div v-else-if="tab === 'cost'" style="padding: 16px;">
            <div :style="{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '10px', marginBottom: '16px' }">
              <KMini label="Total spend" :value="fmtMoney(totalCost, currency)" />
              <KMini label="Input tokens" :value="totalIn.toLocaleString()" />
              <KMini label="Output tokens" :value="totalOut.toLocaleString()" />
            </div>
            <table class="tbl">
              <thead><tr><th>Span</th><th>Provider/Model</th><th class="num">In</th><th class="num">Out</th><th class="num">Cost</th></tr></thead>
              <tbody>
                <tr v-for="s in llmSpans" :key="s.id">
                  <td class="mono" style="font-size: 12px;">{{ s.name }}</td>
                  <td class="mono" style="font-size: 12px; color: var(--text-dim);">{{ s.provider }}/{{ s.model }}</td>
                  <td class="num mono" style="font-size: 12px;">{{ s.inputTokens?.toLocaleString() || '—' }}</td>
                  <td class="num mono" style="font-size: 12px;">{{ s.outputTokens?.toLocaleString() || '—' }}</td>
                  <td class="num mono">{{ fmtMoney(s.cost || 0, currency) }}</td>
                </tr>
              </tbody>
            </table>
            <div style="font-size: 11px; color: var(--text-faint); margin-top: 10px; font-family: var(--font-mono);">
              price book: kave/2026-04-01 · fx: 1 USD = 60,000 IRT (operator-set, 2h ago)
            </div>
          </div>

          <div v-else-if="tab === 'policy'" style="padding: 16px;">
            <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 14px;">
              <template v-if="selected.status === 'blocked'">
                <KIcon name="circle-slash" :size="18" :style="{ color: 'var(--warning)' }" />
                <strong>Denied by policy</strong>
              </template>
              <template v-else>
                <KIcon name="circle-check" :size="18" :style="{ color: 'var(--success)' }" />
                <strong>Allowed by policy</strong>
              </template>
              <span class="mono" style="font-size: 12px; color: var(--text-dim);">{{ selected.policy }}</span>
            </div>
            <KKv k="matched rule" mono>connectors.{{ selected.provider }}.chat_completions</KKv>
            <KKv k="budget impact" mono>+{{ fmtMoney(selected.spend) }} of $25.00 cap</KKv>
            <KKv k="trace input" mono>true (redact: secrets)</KKv>
            <KKv k="trace output" mono>true</KKv>
            <KKv k="retention" mono>30 days</KKv>
            <div style="margin-top: 14px;">
              <div class="sh" style="margin-bottom: 6px;">Casbin policy</div>
              <pre class="code">p, {{ selected.policy }}, {{ selected.provider }}.chat.completions, allow
p, {{ selected.policy }}, {{ selected.provider }}.embeddings, allow
p, {{ selected.policy }}, github.repos.read, allow
p, {{ selected.policy }}, github.repos.write, deny
p, {{ selected.policy }}, *, deny</pre>
            </div>
          </div>

          <div v-else-if="tab === 'i/o'" style="padding: 16px; display: flex; flex-direction: column; gap: 12px;">
            <div>
              <div class="sh" style="margin-bottom: 6px;">Input</div>
              <pre class="code">{{ inputJson }}</pre>
            </div>
            <div>
              <div class="sh" style="margin-bottom: 6px;">Output</div>
              <pre class="code">{{ outputJson }}</pre>
            </div>
          </div>

          <pre v-else-if="tab === 'raw'" class="code" style="margin: 16px;">{{ JSON.stringify({ run: selected }, null, 2) }}</pre>
        </div>
      </aside>
    </template>
  </div>
</template>
