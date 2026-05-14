<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAgents, useCreateAgent } from '@/composables/api/useAgents'
import { usePolicies } from '@/composables/api/usePolicies'
import { useRuns } from '@/composables/api/useRuns'
import { useAgentTokens, useCreateAgentToken } from '@/composables/api/useControl'
import { envId, projectId } from '@/stores/workspace'
import type { Agent } from '@/types/api'
import { asNumber, fmtMoney, fmtRel, fmtMs } from '@/lib/format'
import { KIcon, KBtn, KCard, KStatusDot, KStatusBadge, KCopyBtn, KEmptyState, KKv, KMini } from '@/components/kv'

const currency = 'USD'
const agentsQuery = useAgents(envId)
const policiesQuery = usePolicies(envId)
const runsQuery = useRuns({ projectId, envId, limit: 500 })
const createAgent = useCreateAgent()
const createToken = useCreateAgentToken()

const selected = ref<Agent | null>(null)
const showCreate = ref(false)
const showToken = ref(false)
const tab = ref<'overview' | 'runs' | 'tokens' | 'policy' | 'budget' | 'metadata'>('overview')
const newName = ref('')
const newPolicy = ref('')
const rawToken = ref('')
const tokenName = ref('dashboard token')

const agents = computed(() => agentsQuery.data.value ?? [])
const policies = computed(() => policiesQuery.data.value ?? [])
const agentRuns = computed(() => selected.value ? (runsQuery.data.value ?? []).filter(r => r.agent_id === selected.value?.id) : [])
const selectedTokenQuery = useAgentTokens(computed(() => selected.value?.id ?? ''))
const tokens = computed(() => selectedTokenQuery.data.value ?? [])

function policyName(id?: string) {
  return policies.value.find(p => p.id === id)?.name || id || '-'
}

function pctClass(pct: number) { return pct > 0.9 ? 'danger' : pct > 0.7 ? 'warn' : '' }
function spentFor(agent: Agent) { return (runsQuery.data.value ?? []).filter(r => r.agent_id === agent.id).reduce((sum, r) => sum + asNumber(r.spent), 0) }
function budgetFor(agent: Agent) { return asNumber(agent.monthly_budget) }

async function submitCreateAgent() {
  if (!newName.value.trim()) return
  const agent = await createAgent.mutateAsync({ project_id: projectId.value, env_id: envId.value, name: newName.value.trim(), policy_id: newPolicy.value || undefined })
  selected.value = agent
  newName.value = ''
  newPolicy.value = ''
  showCreate.value = false
}

async function issueToken() {
  if (!selected.value) return
  const result = await createToken.mutateAsync({ agentId: selected.value.id, name: tokenName.value || 'dashboard token' })
  rawToken.value = result.raw_token
  showToken.value = true
}
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div>
        <h1>Agents</h1>
        <p>The actors authorized to run through Kave. Each holds a token and a policy.</p>
      </div>
      <div class="toolbar">
        <KBtn variant="primary" icon="plus" size="sm" @click="showCreate = true">New agent</KBtn>
      </div>
    </div>

    <div class="card" style="overflow: hidden;">
      <table class="tbl">
        <thead><tr>
          <th style="width: 28px"></th><th>Agent</th><th>Policy</th><th class="num">Budget</th><th class="num">Spend</th><th class="num">Runs</th><th class="num">Tokens</th><th>Updated</th><th></th>
        </tr></thead>
        <tbody>
          <tr v-for="a in agents" :key="a.id" @click="selected = a; tab = 'overview'">
            <td><KStatusDot :status="a.status" /></td>
            <td><div style="font-weight: 500;">{{ a.name }}</div><div class="mono" style="font-size: 11px; color: var(--text-faint);">{{ a.id }}</div></td>
            <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ policyName(a.policy_id) }}</td>
            <td class="num mono" style="font-size: 12px;">{{ budgetFor(a) ? fmtMoney(budgetFor(a), currency) + '/mo' : '-' }}</td>
            <td class="num mono" style="min-width: 160px;">
              <div style="display: flex; align-items: center; gap: 8px; justify-content: flex-end;">
                <span style="font-size: 12px;">{{ fmtMoney(spentFor(a), currency) }}</span>
                <div class="prog" style="width: 56px;"><div :class="['fill', pctClass(budgetFor(a) ? spentFor(a) / budgetFor(a) : 0)]" :style="{ width: (budgetFor(a) ? Math.min(spentFor(a) / budgetFor(a) * 100, 100) : 0) + '%' }" /></div>
              </div>
            </td>
            <td class="num">{{ (runsQuery.data.value ?? []).filter(r => r.agent_id === a.id).length }}</td>
            <td class="num">-</td>
            <td style="color: var(--text-muted); font-size: 12px;">{{ fmtRel(a.updated_at) }}</td>
            <td><KBtn size="sm" variant="ghost" icon="chevron-right" /></td>
          </tr>
          <tr v-if="agents.length === 0"><td colspan="9"><KEmptyState icon="bot" title="No agents yet">Create an agent or apply `kave.yaml` resources to start routing traffic.</KEmptyState></td></tr>
        </tbody>
      </table>
    </div>

    <template v-if="selected">
      <div class="drawer-overlay" @click="selected = null" />
      <aside class="drawer" role="dialog" aria-label="Agent detail">
        <header class="drawer-h">
          <KIcon name="bot" :size="18" :style="{ color: 'var(--accent)' }" />
          <div style="flex: 1; min-width: 0;">
            <div class="title" style="display: flex; align-items: center; gap: 8px;">{{ selected.name }}<KCopyBtn :value="selected.id" :label="selected.id" /></div>
            <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px; display: flex; align-items: center; gap: 6px;"><KStatusBadge :status="selected.status" /> policy <span class="mono">{{ policyName(selected.policy_id) }}</span></div>
          </div>
          <KBtn variant="ghost" icon="x" aria-label="Close" @click="selected = null" />
        </header>

        <div class="tabs"><button v-for="t in (['overview','runs','tokens','policy','budget','metadata'] as const)" :key="t" :class="['tab', tab === t ? 'active' : '']" @click="tab = t">{{ t }}</button></div>

        <div class="drawer-b">
          <div v-if="tab === 'overview'" style="padding: 16px;">
            <div :style="{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: '10px', marginBottom: '16px' }">
              <KMini label="Spend" :value="fmtMoney(spentFor(selected), currency)" />
              <KMini label="Budget" :value="budgetFor(selected) ? fmtMoney(budgetFor(selected), currency) : '-'" />
              <KMini label="Runs" :value="agentRuns.length" />
              <KMini label="Tokens" :value="tokens.length" />
            </div>
            <KCard title="Recent runs" flush>
              <table class="tbl"><tbody>
                <tr v-for="r in agentRuns.slice(0, 8)" :key="r.id"><td><KStatusDot :status="r.status" /></td><td>{{ r.name || r.id }}</td><td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ r.id }}</td><td class="num mono" style="font-size: 12px;">{{ fmtMs(r.ended_at ? r.ended_at - r.started_at : null) }}</td><td class="num mono" style="font-size: 12px;">{{ fmtMoney(r.spent, currency) }}</td></tr>
                <tr v-if="agentRuns.length === 0"><td colspan="5"><KEmptyState icon="activity" title="No runs yet">This agent has not executed through Kave.</KEmptyState></td></tr>
              </tbody></table>
            </KCard>
          </div>

          <div v-else-if="tab === 'runs'" style="padding: 16px;"><table class="tbl"><tbody><tr v-for="r in agentRuns" :key="r.id"><td><KStatusDot :status="r.status" /></td><td>{{ r.name || r.id }}</td><td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ r.id }}</td><td class="num mono" style="font-size: 12px;">{{ fmtMoney(r.spent, currency) }}</td></tr></tbody></table></div>

          <div v-else-if="tab === 'tokens'" style="padding: 16px;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;"><div><div style="font-weight: 600;">Agent tokens</div><div style="font-size: 12px; color: var(--text-dim);">Tokens are shown only at creation.</div></div><KBtn variant="primary" size="sm" icon="key" @click="issueToken">Issue token</KBtn></div>
            <table class="tbl"><thead><tr><th>Name</th><th>Prefix</th><th>Created</th><th>Status</th></tr></thead><tbody><tr v-for="t in tokens" :key="t.id"><td>{{ t.name }}</td><td class="mono" style="font-size: 12px;">{{ t.token_prefix }}</td><td style="font-size: 12px; color: var(--text-muted);">{{ fmtRel(t.created_at) }}</td><td><KStatusBadge :status="t.status" /></td></tr><tr v-if="tokens.length === 0"><td colspan="4"><KEmptyState icon="key" title="No tokens">Issue a token to authenticate this agent.</KEmptyState></td></tr></tbody></table>
          </div>

          <div v-else-if="tab === 'policy'" style="padding: 16px;"><KKv k="policy" mono>{{ selected.policy_id || '-' }}</KKv><KKv k="policy name">{{ policyName(selected.policy_id) }}</KKv></div>
          <div v-else-if="tab === 'budget'" style="padding: 16px;"><KMini label="Monthly cap" :value="budgetFor(selected) ? fmtMoney(budgetFor(selected), currency) : 'unlimited'" /></div>
          <pre v-else-if="tab === 'metadata'" class="code" style="margin: 16px;">{{ JSON.stringify(selected.metadata, null, 2) }}</pre>
        </div>
      </aside>
    </template>

    <template v-if="showToken && selected">
      <div class="drawer-overlay" :style="{ zIndex: 60 }" @click="showToken = false" />
      <div role="dialog" class="modal-panel modal-panel-md">
        <strong style="font-size: 15px;">Token created</strong>
        <div style="font-size: 13px; color: var(--text-muted); margin: 6px 0 14px;">Copy this token now. Kave will not show it again.</div>
        <div class="code" style="word-break: break-all; font-size: 12px; padding: 14px; position: relative;">{{ rawToken }}<div style="position: absolute; top: 8px; right: 8px;"><KCopyBtn :value="rawToken" label="copy token" /></div></div>
        <div class="modal-actions"><KBtn @click="showToken = false">Done</KBtn></div>
      </div>
    </template>

    <template v-if="showCreate">
      <div class="drawer-overlay" :style="{ zIndex: 60 }" @click="showCreate = false" />
      <div role="dialog" class="modal-panel modal-panel-sm">
        <strong style="font-size: 15px;">Create agent</strong>
        <div style="font-size: 13px; color: var(--text-muted); margin-top: 4px; margin-bottom: 16px;">Define what this agent is and what policy governs it.</div>
        <div style="display: flex; flex-direction: column; gap: 12px;"><label style="display: flex; flex-direction: column; gap: 4px;"><span class="sh">Name</span><input v-model="newName" class="input" placeholder="my-agent" /></label><label style="display: flex; flex-direction: column; gap: 4px;"><span class="sh">Policy</span><select v-model="newPolicy" class="input select"><option value="">No policy</option><option v-for="p in policies" :key="p.id" :value="p.id">{{ p.name }} ({{ p.id }})</option></select></label></div>
        <div class="modal-actions"><KBtn @click="showCreate = false">Cancel</KBtn><KBtn variant="primary" icon="check" :disabled="createAgent.isPending.value" @click="submitCreateAgent">Create agent</KBtn></div>
      </div>
    </template>
  </div>
</template>
