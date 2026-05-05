<script setup lang="ts">
import { computed } from 'vue'
import { useAgents } from '@/composables/api/useAgents'
import { usePolicies } from '@/composables/api/usePolicies'
import { useCostSummary } from '@/composables/api/useCost'
import { envId } from '@/stores/workspace'
import { asNumber, fmtMoney } from '@/lib/format'
import { KIcon, KBtn, KCard, KStatusBadge, KSparkline, KKv, KEmptyState } from '@/components/kv'

const currency = 'USD'
const agentsQuery = useAgents(envId)
const policiesQuery = usePolicies(envId)
const costQuery = useCostSummary()

const agents = computed(() => agentsQuery.data.value ?? [])
const policies = computed(() => policiesQuery.data.value ?? [])
const spend = computed(() => costQuery.data.value)
const totalSpend = computed(() => asNumber(spend.value?.total))
const topAgentId = computed(() => Object.entries(spend.value?.by_agent ?? {}).sort((a, b) => asNumber(b[1]) - asNumber(a[1]))[0]?.[0] ?? '')
const topAgent = computed(() => agents.value.find(a => a.id === topAgentId.value))

function policyOf(id?: string) { return policies.value.find(p => p.id === id) }
function pctClass(pct: number) { return pct > 0.9 ? 'danger' : pct > 0.7 ? 'warn' : '' }
function spentFor(id: string) { return asNumber(spend.value?.by_agent[id]) }
function budgetFor(id: string) { return asNumber(agents.value.find(a => a.id === id)?.monthly_budget) }
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div><h1>Cost &amp; Budgets</h1><p>Where money goes, and what limits are enforced.</p></div>
      <div class="toolbar"><KBtn icon="rotate" size="sm" @click="costQuery.refetch()">Refresh</KBtn><KBtn icon="file-text" size="sm">Export</KBtn></div>
    </div>

    <section :style="{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(190px,1fr))', gap: '10px' }">
      <div class="stat"><div class="label">Total spend</div><div class="val">{{ fmtMoney(totalSpend, currency) }}</div><div class="delta">daemon spend report</div><KSparkline :data="[0,0,0,0,0,totalSpend]" /></div>
      <div class="stat"><div class="label">Agents with spend</div><div class="val">{{ Object.keys(spend?.by_agent ?? {}).length }}</div><div class="delta">current period</div></div>
      <div class="stat"><div class="label">Connectors metered</div><div class="val">{{ Object.keys(spend?.by_connector ?? {}).length }}</div><div class="delta">current period</div></div>
      <div class="stat"><div class="label">Top spender</div><div class="val" style="font-size: 16px;">{{ topAgent?.name || '-' }}</div><div class="delta">{{ fmtMoney(spend?.by_agent?.[topAgentId] ?? 0, currency) }}</div></div>
    </section>

    <KCard title="Budget usage" subtitle="Per-agent caps and current consumption" flush>
      <table class="tbl">
        <thead><tr><th>Agent</th><th>Policy</th><th class="num">Cap</th><th class="num">Spent</th><th class="num">Remaining</th><th>Behavior</th><th style="width: 200px;">Usage</th></tr></thead>
        <tbody>
          <tr v-for="a in agents" :key="a.id">
            <td>{{ a.name }}</td><td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ a.policy_id || '-' }}</td>
            <td class="num mono" style="font-size: 12px;">{{ budgetFor(a.id) ? fmtMoney(budgetFor(a.id), currency) : 'unlimited' }}</td>
            <td class="num mono" style="font-size: 12px;">{{ fmtMoney(spentFor(a.id), currency) }}</td>
            <td class="num mono" style="font-size: 12px;">{{ budgetFor(a.id) ? fmtMoney(Math.max(0, budgetFor(a.id) - spentFor(a.id)), currency) : '-' }}</td>
            <td><KStatusBadge :status="policyOf(a.policy_id)?.budget_behavior || 'observe'" /></td>
            <td><div style="display: flex; align-items: center; gap: 8px;"><div class="prog" style="flex: 1;"><div :class="['fill', pctClass(budgetFor(a.id) ? spentFor(a.id) / budgetFor(a.id) : 0)]" :style="{ width: (budgetFor(a.id) ? Math.min(spentFor(a.id) / budgetFor(a.id) * 100, 100) : 0) + '%' }" /></div><span style="font-size: 11px; color: var(--text-muted); width: 36px; text-align: right;">{{ budgetFor(a.id) ? Math.round(spentFor(a.id) / budgetFor(a.id) * 100) : 0 }}%</span></div></td>
          </tr>
          <tr v-if="agents.length === 0"><td colspan="7"><KEmptyState icon="wallet" title="No agent budgets yet">Create agents and configure monthly budgets to track usage here.</KEmptyState></td></tr>
        </tbody>
      </table>
    </KCard>

    <section :style="{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: '16px' }">
      <KCard title="Spend by connector" subtitle="From daemon spend report"><KKv v-for="(value, key) in spend?.by_connector ?? {}" :key="key" :k="key" mono>{{ fmtMoney(value, currency) }}</KKv><KEmptyState v-if="Object.keys(spend?.by_connector ?? {}).length === 0" icon="plug" title="No connector spend">Metered connectors will appear after traffic is proxied.</KEmptyState></KCard>
      <KCard title="Spend by model" subtitle="From daemon spend report"><KKv v-for="(value, key) in spend?.by_model ?? {}" :key="key" :k="key" mono>{{ fmtMoney(value, currency) }}</KKv><KEmptyState v-if="Object.keys(spend?.by_model ?? {}).length === 0" icon="brain" title="No model spend">Model breakdown will appear after LLM calls are metered.</KEmptyState></KCard>
    </section>
  </div>
</template>
