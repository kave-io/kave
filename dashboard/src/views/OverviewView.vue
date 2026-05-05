<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardOverview } from '@/composables/api/useOverview'
import { envId, projectId } from '@/stores/workspace'
import { asNumber, fmtMoney } from '@/lib/format'
import { KIcon, KBtn, KBadge, KCard, KSparkline, KLiveStream, KEmptyState } from '@/components/kv'

const router = useRouter()
const currency = 'USD'

const overviewQuery = useDashboardOverview({ projectId, envId, recentLimit: 12 })
const overview = computed(() => overviewQuery.data.value)

const stats = computed(() => {
  const data = overview.value
  return [
    { label: 'Runs', value: String(data?.total_runs ?? 0), delta: 'aggregate RPC', up: true, spark: [0,0,0,0,0,0,0,0,0,0,0,data?.total_runs ?? 0] },
    { label: 'Active runs', value: String(data?.active_runs ?? 0), delta: data?.active_runs ? 'in progress' : 'none active', up: true, spark: [0,0,0,0,0,0,0,0,0,0,0,data?.active_runs ?? 0] },
    { label: 'Spend', value: fmtMoney(data?.spend.total ?? '0', currency), delta: 'reported by daemon', up: true, spark: [0,0,0,0,0,0,0,0,0,0,0,asNumber(data?.spend.total)] },
    { label: 'Blocked runs', value: String(data?.blocked_runs ?? 0), delta: data?.blocked_runs ? 'needs review' : 'none', up: (data?.blocked_runs ?? 0) === 0, spark: [0,0,0,0,0,0,0,0,0,0,0,data?.blocked_runs ?? 0] },
    { label: 'Errors', value: String(data?.failed_runs ?? 0), delta: data?.failed_runs ? 'failed runs' : 'none', up: (data?.failed_runs ?? 0) === 0, spark: [0,0,0,0,0,0,0,0,0,0,0,data?.failed_runs ?? 0] },
    { label: 'Tokens', value: `${(((data?.total_input_tokens ?? 0) + (data?.total_output_tokens ?? 0)) / 1000).toFixed(1)}k`, delta: 'input + output', up: true, spark: [0,0,0,0,0,0,0,0,0,0,0,(data?.total_input_tokens ?? 0) + (data?.total_output_tokens ?? 0)] },
  ]
})

const recentBlocks = computed(() => (overview.value?.recent_attention_runs ?? [])
  .map(r => ({
    agent: r.agent_id || 'unknown agent',
    reason: r.error_message || r.status,
    policy: r.policy_id || 'no policy',
    when: new Date(r.updated_at || r.started_at).toISOString(),
  })))

const spendByProvider = computed(() => {
  const entries = Object.entries(overview.value?.spend.by_connector ?? {}).map(([name, total]) => ({ name, cost: asNumber(total) }))
  const max = entries.reduce((acc, item) => Math.max(acc, item.cost), 0)
  return entries.map(item => ({ ...item, share: max > 0 ? item.cost / max : 0 })).slice(0, 6)
})

const topAgents = computed(() => overview.value?.top_agents ?? [])
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
        <KEmptyState v-if="recentBlocks.length === 0" icon="shield-check" title="No blocked or failed runs">
          When policy or budget blocks happen, they will appear here.
        </KEmptyState>
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
        <KEmptyState v-if="spendByProvider.length === 0" icon="wallet" title="No spend recorded">
          Cost data will appear after proxied LLM calls are metered.
        </KEmptyState>
      </KCard>

      <KCard title="Top agents by spend" subtitle="This month" flush>
        <table class="tbl">
          <thead><tr><th>Agent</th><th class="num">Runs</th><th class="num">Spend</th></tr></thead>
          <tbody>
            <tr v-for="a in topAgents" :key="a.agent_id" @click="router.push('/agents')">
              <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                  <KIcon name="bot" :size="14" :style="{ color: 'var(--text-dim)' }" />
                  <span style="font-weight: 500;">{{ a.agent_name || a.agent_id }}</span>
                </div>
              </td>
              <td class="num mono" style="font-size: 12px;">{{ a.run_count }}</td>
              <td class="num">{{ fmtMoney(a.spend, currency) }}</td>
            </tr>
            <tr v-if="topAgents.length === 0"><td colspan="3">
              <KEmptyState icon="bot" title="No agents yet">Create an agent or apply config resources to populate this table.</KEmptyState>
            </td></tr>
          </tbody>
        </table>
      </KCard>
    </section>
  </div>
</template>
