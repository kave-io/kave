<script setup lang="ts">
import { useRouter } from 'vue-router'
import { AGENTS, fmtMoney } from '@/data/mock'
import { KIcon, KBtn, KBadge, KCard, KSparkline, KLiveStream } from '@/components/kv'

const router = useRouter()
const currency = 'USD'

const stats = [
  { label: 'Active runs',     value: '7',     delta: '+3 last hour', up: true,  spark: [3,5,4,6,5,7,9,7,6,8,7,7]   },
  { label: 'Spend (24h)',     value: fmtMoney(12.84, currency), delta: '+18%', up: true, spark: [1,2,2,3,4,4,6,8,9,8,10,12] },
  { label: 'Blocked actions', value: '3',     delta: 'budget cap',   up: false, spark: [0,0,1,0,1,0,2,1,0,1,3,3]   },
  { label: 'Avg latency p50', value: '842ms', delta: '−7%',          up: true,  spark: [9,9,8,9,8,8,7,8,8,7,7,8]   },
  { label: 'Errors (24h)',    value: '14',    delta: '0.4% of runs', up: false, spark: [2,3,1,2,4,2,3,5,2,1,3,4]   },
  { label: 'Tokens (24h)',    value: '2.4M',  delta: '+22%',         up: true,  spark: [2,3,3,4,5,5,7,8,9,11,12,14] },
]

const recentBlocks = [
  { agent: 'scraper-agent', reason: 'budget cap reached',                    policy: 'pol_strict_v1',  when: '4m ago'  },
  { agent: 'invoice-agent', reason: 'method github.repos.write not allowed', policy: 'pol_finance_v1', when: '12m ago' },
  { agent: 'support-bot',   reason: 'cost > $0.10 per call',                 policy: 'pol_support_v2', when: '38m ago' },
]

const spendByProvider = [
  { name: 'openai',    cost: 7.42, share: 0.58 },
  { name: 'anthropic', cost: 3.18, share: 0.25 },
  { name: 'gemini',    cost: 1.52, share: 0.12 },
  { name: 'groq',      cost: 0.72, share: 0.05 },
]

const topAgents = [...AGENTS].sort((a, b) => b.spent - a.spent).slice(0, 4)

function pctClass(pct: number) { return pct > 0.9 ? 'danger' : pct > 0.7 ? 'warn' : '' }
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div>
        <h1>Command Center</h1>
        <p>Local daemon · project <code class="mono" style="color: var(--text-muted);">default</code> / env <code class="mono" style="color: var(--text-muted);">dev</code></p>
      </div>
      <div class="toolbar">
        <KBtn icon="terminal" size="sm">Setup guide</KBtn>
        <KBtn variant="primary" icon="plus" size="sm">New agent</KBtn>
      </div>
    </div>

    <section :style="{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '10px' }">
      <div v-for="s in stats" :key="s.label" class="stat">
        <div class="label">{{ s.label }}</div>
        <div class="val">{{ s.value }}</div>
        <div :class="['delta', s.up ? 'up' : 'down']">{{ s.delta }}</div>
        <KSparkline :data="s.spark" :color="s.up ? 'var(--accent)' : 'var(--warning)'" />
      </div>
    </section>

    <section :style="{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.6fr) minmax(320px, 1fr)', gap: '16px' }">
      <KCard title="Live activity" subtitle="Streaming spans, policy decisions, cost events" flush>
        <template #action>
          <div style="display: flex; gap: 8px; align-items: center;">
            <KBadge tone="success" dot="live">Live</KBadge>
            <KBtn variant="ghost" size="sm" iconRight="arrow-up-right" @click="router.push('/monitor')">Open monitor</KBtn>
          </div>
        </template>
        <KLiveStream :limit="9" compact />
      </KCard>

      <KCard title="Needs attention" subtitle="Recent policy blocks" flush>
        <div
          v-for="(b, i) in recentBlocks"
          :key="i"
          :style="{
            display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '11px 16px',
            borderBottom: i < recentBlocks.length - 1 ? '1px solid var(--border-soft)' : '0',
          }"
        >
          <KIcon name="circle-slash" :size="16" :style="{ color: 'var(--warning)', marginTop: '1px' }" />
          <div style="min-width: 0; flex: 1;">
            <div style="font-size: 13px; font-weight: 500;">{{ b.agent }}</div>
            <div style="font-size: 12px; color: var(--text-dim); margin-top: 1px;">{{ b.reason }}</div>
            <div style="font-size: 11px; color: var(--text-faint); margin-top: 4px; font-family: var(--font-mono);">{{ b.policy }} · {{ b.when }}</div>
          </div>
        </div>
      </KCard>
    </section>

    <section :style="{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: '16px' }">
      <KCard title="Spend by provider" subtitle="Last 24h" flush>
        <div style="padding: 4px 16px 14px;">
          <div
            v-for="p in spendByProvider"
            :key="p.name"
            :style="{ padding: '10px 0', display: 'grid', gridTemplateColumns: '120px 1fr 80px', alignItems: 'center', gap: '12px' }"
          >
            <span style="font-family: var(--font-mono); font-size: 12px; color: var(--text-muted);">{{ p.name }}</span>
            <div class="prog"><div class="fill" :style="{ width: (p.share * 100) + '%' }" /></div>
            <span style="text-align: right; font-variant-numeric: tabular-nums; font-weight: 500;">{{ fmtMoney(p.cost, currency) }}</span>
          </div>
        </div>
      </KCard>

      <KCard title="Top agents by spend" subtitle="This month" flush>
        <table class="tbl">
          <thead><tr><th>Agent</th><th>Policy</th><th class="num">Spend</th><th class="num">Budget</th></tr></thead>
          <tbody>
            <tr v-for="a in topAgents" :key="a.id" @click="router.push('/agents')">
              <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                  <KIcon name="bot" :size="14" :style="{ color: 'var(--text-dim)' }" />
                  <span style="font-weight: 500;">{{ a.name }}</span>
                </div>
              </td>
              <td class="mono" style="font-size: 12px; color: var(--text-dim);">{{ a.policy }}</td>
              <td class="num">{{ fmtMoney(a.spent, currency) }}</td>
              <td class="num" style="min-width: 140px;">
                <div style="display: flex; align-items: center; gap: 8px; justify-content: flex-end;">
                  <div class="prog" style="width: 60px;">
                    <div :class="['fill', pctClass(a.spent / a.budget)]" :style="{ width: Math.min(a.spent / a.budget * 100, 100) + '%' }" />
                  </div>
                  <span style="font-size: 12px; color: var(--text-dim); width: 36px; text-align: right;">{{ Math.round(a.spent / a.budget * 100) }}%</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </KCard>
    </section>
  </div>
</template>
