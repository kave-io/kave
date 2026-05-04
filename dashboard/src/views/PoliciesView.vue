<script setup lang="ts">
import { ref, computed } from 'vue'
import { POLICIES, AGENTS, fmtRel, fmtMoney, type Policy } from '@/data/mock'
import { KIcon, KBtn, KBadge, KCard, KStatusBadge, KKv } from '@/components/kv'

const currency = 'USD'

const selected = ref<Policy>(POLICIES[0]!)
const tab = ref<'summary' | 'rules' | 'budget' | 'tracing' | 'test' | 'raw'>('summary')

const llmConnectors = computed(() => selected.value.connectors.filter(c => ['openai','anthropic','gemini'].includes(c)))
const toolConnectors = computed(() => selected.value.connectors.filter(c => !['openai','anthropic','gemini'].includes(c)))
const attachedAgents = computed(() => AGENTS.filter(a => a.policy === selected.value.id))

// simulator state
const simConn = ref('openai')
const simMethod = ref('chat.completions')
const simModel = ref('gpt-4.1-mini')
const simResult = ref<{ allowed: boolean; reason: string; cost: number } | null>(null)

function runSim() {
  const allowed = selected.value.connectors.includes(simConn.value) && simMethod.value !== 'repos.write'
  simResult.value = {
    allowed,
    reason: allowed
      ? `Matched rule: connectors.${simConn.value}.${simMethod.value.replace('.', '_')} = allow`
      : `Method ${simConn.value}.${simMethod.value} is not in the allow-list.`,
    cost: 0.0021,
  }
}
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px; height: 100%;">
    <div class="page-h">
      <div>
        <h1>Policies</h1>
        <p>Define what each agent is allowed to do. Test before you deploy.</p>
      </div>
      <div class="toolbar">
        <KBtn variant="primary" icon="plus" size="sm">New policy</KBtn>
      </div>
    </div>

    <div :style="{ display: 'grid', gridTemplateColumns: 'minmax(260px, 320px) minmax(0,1fr)', gap: '12px', flex: 1, minHeight: 0 }">
      <div class="card" style="overflow: auto;">
        <button
          v-for="p in POLICIES"
          :key="p.id"
          :style="{
            display: 'block', width: '100%', textAlign: 'left', padding: '12px 14px', border: 0,
            background: selected.id === p.id ? 'var(--secondary-tint)' : 'transparent',
            borderBottom: '1px solid var(--border-soft)', cursor: 'pointer',
            borderLeft: selected.id === p.id ? '2px solid var(--secondary)' : '2px solid transparent',
            fontFamily: 'inherit',
          }"
          @click="selected = p"
        >
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <strong style="font-size: 13px;">{{ p.name }}</strong>
            <KStatusBadge :status="p.mode" />
          </div>
          <div class="mono" style="font-size: 11px; color: var(--text-faint); margin-top: 3px;">{{ p.id }}</div>
          <div style="font-size: 12px; color: var(--text-muted); margin-top: 6px; display: flex; gap: 10px;">
            <span>{{ p.connectors.length }} connectors</span>
            <span>{{ p.agents }} agents</span>
            <span v-if="p.budget">{{ fmtMoney(p.budget, currency) }}/mo</span>
          </div>
        </button>
      </div>

      <div class="card" style="overflow: hidden; display: flex; flex-direction: column;">
        <div class="card-h">
          <div>
            <h3>{{ selected.name }}</h3>
            <div class="sub mono" style="font-size: 12px;">{{ selected.id }} · updated {{ fmtRel(selected.updated) }}</div>
          </div>
          <div style="display: flex; gap: 6px;">
            <KBtn size="sm" icon="play">Test</KBtn>
            <KBtn size="sm" variant="ghost" icon="copy">Duplicate</KBtn>
          </div>
        </div>
        <div class="tabs">
          <button v-for="t in (['summary','rules','budget','tracing','test','raw'] as const)" :key="t" :class="['tab', tab === t ? 'active' : '']" @click="tab = t">{{ t }}</button>
        </div>
        <div style="flex: 1; overflow: auto;">
          <div v-if="tab === 'summary'" style="padding: 18px;">
            <div style="font-size: 14px; line-height: 1.7; color: var(--text);">
              This policy <strong>{{ selected.mode === 'enforce' ? 'enforces' : 'observes' }}</strong> the following:
            </div>
            <ul style="margin: 10px 0 0; padding-left: 20px; font-size: 13.5px; color: var(--text-muted); line-height: 1.8;">
              <li>Allows <strong>LLM actions</strong> through {{ llmConnectors.join(', ') || 'no providers' }}.</li>
              <li>Allows <strong>tool actions</strong> through {{ toolConnectors.join(', ') || 'no tools' }}.</li>
              <li v-if="selected.budget">Caps spend at <strong>{{ fmtMoney(selected.budget, currency) }}/month</strong>.</li>
              <li v-if="selected.behavior === 'block'"><strong>Blocks</strong> when budget is exceeded.</li>
              <li v-else><strong>Warns</strong> at 90% budget; does not block.</li>
              <li>Records input + output traces (with secret redaction) for 30 days.</li>
            </ul>
            <div style="margin-top: 18px;">
              <div class="sh" style="margin-bottom: 8px;">Attached agents · {{ selected.agents }}</div>
              <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                <KBadge v-for="a in attachedAgents" :key="a.id" tone="neutral" icon="bot">{{ a.name }}</KBadge>
                <span v-if="selected.agents === 0" style="font-size: 12px; color: var(--text-faint);">No agents attached.</span>
              </div>
            </div>
          </div>

          <div v-else-if="tab === 'rules'" style="padding: 16px; display: flex; flex-direction: column; gap: 14px;">
            <KCard title="Allowed connectors" flush>
              <div style="padding: 14px; display: flex; flex-wrap: wrap; gap: 6px;">
                <KBadge v-for="c in selected.connectors" :key="c" tone="accent" icon="plug">{{ c }}</KBadge>
              </div>
            </KCard>
            <KCard title="Method rules" flush>
              <table class="tbl">
                <thead><tr><th>Connector</th><th>Method</th><th>Decision</th></tr></thead>
                <tbody>
                  <tr><td class="mono" style="font-size: 12px;">openai</td><td class="mono" style="font-size: 12px;">chat.completions</td><td><KBadge tone="success">allow</KBadge></td></tr>
                  <tr><td class="mono" style="font-size: 12px;">openai</td><td class="mono" style="font-size: 12px;">embeddings</td><td><KBadge tone="success">allow</KBadge></td></tr>
                  <tr><td class="mono" style="font-size: 12px;">github</td><td class="mono" style="font-size: 12px;">repos.read</td><td><KBadge tone="success">allow</KBadge></td></tr>
                  <tr><td class="mono" style="font-size: 12px;">github</td><td class="mono" style="font-size: 12px;">repos.write</td><td><KBadge tone="danger">deny</KBadge></td></tr>
                  <tr><td class="mono" style="font-size: 12px; color: var(--text-faint);">*</td><td class="mono" style="font-size: 12px; color: var(--text-faint);">*</td><td><KBadge tone="danger">deny (default)</KBadge></td></tr>
                </tbody>
              </table>
            </KCard>
          </div>

          <div v-else-if="tab === 'budget'" style="padding: 16px;">
            <KKv k="cap" mono>{{ selected.budget ? fmtMoney(selected.budget, currency) : 'unlimited' }}</KKv>
            <KKv k="period">monthly</KKv>
            <KKv k="behavior">{{ selected.behavior }}</KKv>
          </div>

          <div v-else-if="tab === 'tracing'" style="padding: 16px;">
            <KKv k="trace input">enabled (redact: secrets, pii)</KKv>
            <KKv k="trace output">enabled</KKv>
            <KKv k="retention">30 days</KKv>
            <KKv k="sampling">100%</KKv>
          </div>

          <div v-else-if="tab === 'test'" style="padding: 18px;">
            <div style="font-size: 13px; color: var(--text-muted); margin-bottom: 14px;">Simulate a request and see whether this policy would allow or deny it.</div>
            <div :style="{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: '10px', marginBottom: '14px' }">
              <label style="display: flex; flex-direction: column; gap: 4px;">
                <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Connector</span>
                <select v-model="simConn" class="input select">
                  <option v-for="c in ['openai','anthropic','github','postgres','gmail','stripe']" :key="c">{{ c }}</option>
                </select>
              </label>
              <label style="display: flex; flex-direction: column; gap: 4px;">
                <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Method</span>
                <select v-model="simMethod" class="input select">
                  <option v-for="m in ['chat.completions','embeddings','repos.read','repos.write','query']" :key="m">{{ m }}</option>
                </select>
              </label>
              <label style="display: flex; flex-direction: column; gap: 4px;">
                <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Model</span>
                <input v-model="simModel" class="input mono" />
              </label>
            </div>
            <KBtn variant="primary" icon="play" @click="runSim">Run simulation</KBtn>
            <div
              v-if="simResult"
              :style="{
                marginTop: '18px', padding: '16px', borderRadius: '8px',
                background: simResult.allowed ? 'var(--success-tint)' : 'var(--danger-tint)',
                border: '1px solid ' + (simResult.allowed ? 'rgba(31,122,77,0.25)' : 'rgba(179,38,30,0.25)'),
              }"
            >
              <div style="display: flex; align-items: center; gap: 10px;">
                <KIcon :name="simResult.allowed ? 'circle-check' : 'circle-slash'" :size="20" :style="{ color: simResult.allowed ? 'var(--success)' : 'var(--danger)' }" />
                <strong :style="{ fontSize: '15px', color: simResult.allowed ? 'var(--success)' : 'var(--danger)' }">{{ simResult.allowed ? 'Allowed' : 'Denied' }}</strong>
              </div>
              <div style="margin-top: 8px; font-size: 13px; color: var(--text);">{{ simResult.reason }}</div>
              <div v-if="simResult.allowed" class="mono" style="margin-top: 6px; font-size: 12px; color: var(--text-muted);">est. cost {{ fmtMoney(simResult.cost) }} · budget impact 0.008%</div>
            </div>
          </div>

          <pre v-else-if="tab === 'raw'" class="code" style="margin: 16px;">{{ JSON.stringify(selected, null, 2) }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
