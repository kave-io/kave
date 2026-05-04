<script setup lang="ts">
import { ref, computed } from 'vue'
import { AGENTS, RUNS, POLICIES, fmtRel, fmtMs, fmtMoney, type Agent } from '@/data/mock'
import { KIcon, KBtn, KCard, KStatusDot, KStatusBadge, KCopyBtn, KEmptyState, KKv, KMini } from '@/components/kv'

const currency = 'USD'

const selected = ref<Agent | null>(null)
const showCreate = ref(false)
const showToken = ref(false)
const tab = ref<'overview' | 'runs' | 'tokens' | 'policy' | 'budget' | 'metadata'>('overview')

const newName = ref('')
const newPolicy = ref('pol_research_v3')
const newBudget = ref('25')

function pctClass(pct: number) { return pct > 0.9 ? 'danger' : pct > 0.7 ? 'warn' : '' }

const agentRuns = computed(() => selected.value ? RUNS.filter(r => r.agentId === selected.value!.id) : [])

const tokenForModal = computed(() => selected.value
  ? `kv_${selected.value.id.slice(4)}_${Math.random().toString(36).slice(2, 10)}_a8f3${Math.random().toString(36).slice(2, 14)}`
  : '')

const metadata = computed(() => selected.value
  ? JSON.stringify({ id: selected.value.id, name: selected.value.name, env: 'dev', team: 'platform', created_by: 'you@local' }, null, 2)
  : '')
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
          <th style="width: 28px"></th>
          <th>Agent</th><th>Policy</th><th class="num">Budget</th><th class="num">Spend</th><th class="num">24h runs</th><th class="num">Tokens</th><th>Last seen</th><th></th>
        </tr></thead>
        <tbody>
          <tr v-for="a in AGENTS" :key="a.id" @click="selected = a; tab = 'overview'">
            <td><KStatusDot :status="a.status" /></td>
            <td>
              <div style="font-weight: 500;">{{ a.name }}</div>
              <div class="mono" style="font-size: 11px; color: var(--text-faint);">{{ a.id }}</div>
            </td>
            <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ a.policy }}</td>
            <td class="num mono" style="font-size: 12px;">{{ fmtMoney(a.budget, currency) }}/mo</td>
            <td class="num mono" style="min-width: 160px;">
              <div style="display: flex; align-items: center; gap: 8px; justify-content: flex-end;">
                <span style="font-size: 12px;">{{ fmtMoney(a.spent, currency) }}</span>
                <div class="prog" style="width: 56px;">
                  <div :class="['fill', pctClass(a.spent / a.budget)]" :style="{ width: Math.min(a.spent / a.budget * 100, 100) + '%' }" />
                </div>
              </div>
            </td>
            <td class="num">{{ a.runs24h }}</td>
            <td class="num">{{ a.tokens }}</td>
            <td style="color: var(--text-muted); font-size: 12px;">{{ fmtRel(a.lastSeen) }}</td>
            <td><KBtn size="sm" variant="ghost" icon="chevron-right" /></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Detail drawer -->
    <template v-if="selected">
      <div class="drawer-overlay" @click="selected = null" />
      <aside class="drawer" role="dialog" aria-label="Agent detail">
        <header class="drawer-h">
          <KIcon name="bot" :size="18" :style="{ color: 'var(--accent)' }" />
          <div style="flex: 1; min-width: 0;">
            <div class="title" style="display: flex; align-items: center; gap: 8px;">
              {{ selected.name }}
              <KCopyBtn :value="selected.id" :label="selected.id" />
            </div>
            <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px; display: flex; align-items: center; gap: 6px;">
              <KStatusBadge :status="selected.status" /> · policy <span class="mono">{{ selected.policy }}</span> · last seen {{ fmtRel(selected.lastSeen) }}
            </div>
          </div>
          <KBtn variant="ghost" icon="x" aria-label="Close" @click="selected = null" />
        </header>

        <div class="tabs">
          <button v-for="t in (['overview','runs','tokens','policy','budget','metadata'] as const)" :key="t" :class="['tab', tab === t ? 'active' : '']" @click="tab = t">{{ t }}</button>
        </div>

        <div class="drawer-b">
          <div v-if="tab === 'overview'" style="padding: 16px;">
            <div :style="{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: '10px', marginBottom: '16px' }">
              <KMini label="Spend (mo)" :value="fmtMoney(selected.spent, currency)" />
              <KMini label="Budget" :value="fmtMoney(selected.budget, currency)" />
              <KMini label="24h runs" :value="selected.runs24h" />
              <KMini label="Tokens" :value="selected.tokens" />
            </div>
            <KCard title="Recent runs" flush>
              <table class="tbl">
                <tbody>
                  <tr v-for="r in agentRuns.slice(0, 5)" :key="r.id">
                    <td><KStatusDot :status="r.status" /></td>
                    <td>{{ r.name }}</td>
                    <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ r.model }}</td>
                    <td class="num mono" style="font-size: 12px;">{{ fmtMs(r.duration) }}</td>
                    <td class="num mono" style="font-size: 12px;">{{ fmtMoney(r.spend, currency) }}</td>
                  </tr>
                  <tr v-if="agentRuns.length === 0"><td colspan="5">
                    <KEmptyState icon="activity" title="No runs yet">This agent hasn't executed any runs.</KEmptyState>
                  </td></tr>
                </tbody>
              </table>
            </KCard>
          </div>

          <div v-else-if="tab === 'runs'" style="padding: 16px;">
            <table class="tbl">
              <tbody>
                <tr v-for="r in agentRuns" :key="r.id">
                  <td><KStatusDot :status="r.status" /></td>
                  <td>{{ r.name }}</td>
                  <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ r.id }}</td>
                  <td class="num mono" style="font-size: 12px;">{{ fmtMs(r.duration) }}</td>
                  <td class="num mono" style="font-size: 12px;">{{ fmtMoney(r.spend, currency) }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="tab === 'tokens'" style="padding: 16px;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
              <div>
                <div style="font-weight: 600;">Agent tokens</div>
                <div style="font-size: 12px; color: var(--text-dim);">Tokens are shown only at creation. Rotate to invalidate.</div>
              </div>
              <KBtn variant="primary" size="sm" icon="key" @click="showToken = true">Issue token</KBtn>
            </div>
            <table class="tbl">
              <thead><tr><th>Fingerprint</th><th>Created</th><th>Last used</th><th>Expires</th><th>Status</th><th></th></tr></thead>
              <tbody>
                <tr>
                  <td class="mono" style="font-size: 12px;">kv_••••••a8f3</td>
                  <td style="font-size: 12px; color: var(--text-muted);">3d ago</td>
                  <td style="font-size: 12px; color: var(--text-muted);">4m ago</td>
                  <td style="font-size: 12px; color: var(--text-muted);">in 87d</td>
                  <td><KStatusBadge status="active" /></td>
                  <td><KBtn size="sm" variant="ghost">Revoke</KBtn></td>
                </tr>
                <tr>
                  <td class="mono" style="font-size: 12px;">kv_••••••2d1c</td>
                  <td style="font-size: 12px; color: var(--text-muted);">14d ago</td>
                  <td style="font-size: 12px; color: var(--text-muted);">2d ago</td>
                  <td style="font-size: 12px; color: var(--text-muted);">in 76d</td>
                  <td><KStatusBadge status="active" /></td>
                  <td><KBtn size="sm" variant="ghost">Revoke</KBtn></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-else-if="tab === 'policy'" style="padding: 16px;">
            <KKv k="policy" mono>{{ selected.policy }}</KKv>
            <KKv k="mode">enforce</KKv>
            <KKv k="behavior">warn</KKv>
            <KKv k="connectors" mono>openai, anthropic, github, postgres</KKv>
          </div>

          <div v-else-if="tab === 'budget'" style="padding: 16px;">
            <KMini label="Monthly cap" :value="fmtMoney(selected.budget, currency)" />
            <div style="margin-top: 14px;">
              <div class="prog" style="height: 10px;">
                <div class="fill" :style="{ width: (selected.spent / selected.budget * 100) + '%', background: selected.spent / selected.budget > 0.9 ? 'var(--danger)' : 'var(--accent)' }" />
              </div>
              <div style="display: flex; justify-content: space-between; font-size: 12px; margin-top: 6px; color: var(--text-muted);">
                <span>{{ fmtMoney(selected.spent, currency) }} spent</span>
                <span>{{ fmtMoney(selected.budget - selected.spent, currency) }} remaining</span>
              </div>
            </div>
          </div>

          <pre v-else-if="tab === 'metadata'" class="code" style="margin: 16px;">{{ metadata }}</pre>
        </div>
      </aside>
    </template>

    <!-- Token modal -->
    <template v-if="showToken && selected">
      <div class="drawer-overlay" :style="{ zIndex: 60 }" @click="showToken = false" />
      <div role="dialog" :style="{ position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: '12px', padding: '20px', width: '540px', zIndex: 61, boxShadow: 'var(--shadow-lg)' }">
        <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 6px;">
          <KIcon name="key" :size="18" :style="{ color: 'var(--accent)' }" />
          <strong style="font-size: 15px;">Token created</strong>
        </div>
        <div style="font-size: 13px; color: var(--text-muted); margin-bottom: 14px;">Copy this token now. Kave will not show it again — only its fingerprint.</div>
        <div class="code" style="word-break: break-all; font-size: 12px; padding: 14px; position: relative;">
          {{ tokenForModal }}
          <div style="position: absolute; top: 8px; right: 8px;"><KCopyBtn :value="tokenForModal" label="copy token" /></div>
        </div>
        <div style="margin-top: 14px; padding: 12px; background: var(--warning-tint); border-radius: 6px; font-size: 12px; color: var(--warning); display: flex; gap: 8px;">
          <KIcon name="circle-alert" :size="14" :style="{ flexShrink: 0, marginTop: '2px' }" />
          <span>This token grants access scoped to <strong>{{ selected.name }}</strong> under policy <span class="mono">{{ selected.policy }}</span>. Treat it like a secret.</span>
        </div>
        <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px;">
          <KBtn @click="showToken = false">Done</KBtn>
        </div>
      </div>
    </template>

    <!-- Create modal -->
    <template v-if="showCreate">
      <div class="drawer-overlay" :style="{ zIndex: 60 }" @click="showCreate = false" />
      <div role="dialog" :style="{ position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: '12px', padding: '22px', width: '520px', zIndex: 61, boxShadow: 'var(--shadow-lg)' }">
        <strong style="font-size: 15px;">Create agent</strong>
        <div style="font-size: 13px; color: var(--text-muted); margin-top: 4px; margin-bottom: 16px;">Define what this agent is and what policy governs it.</div>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          <label style="display: flex; flex-direction: column; gap: 4px;">
            <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Name</span>
            <input v-model="newName" class="input" placeholder="my-agent" />
          </label>
          <label style="display: flex; flex-direction: column; gap: 4px;">
            <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Policy</span>
            <select v-model="newPolicy" class="input select">
              <option v-for="p in POLICIES" :key="p.id" :value="p.id">{{ p.name }} ({{ p.id }})</option>
            </select>
          </label>
          <label style="display: flex; flex-direction: column; gap: 4px;">
            <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Monthly budget (USD)</span>
            <input v-model="newBudget" class="input mono" />
          </label>
          <label style="display: flex; flex-direction: column; gap: 4px;">
            <span style="font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-dim);">Environment</span>
            <select class="input select"><option>dev</option><option>staging</option><option>prod</option></select>
          </label>
        </div>
        <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 18px;">
          <KBtn @click="showCreate = false">Cancel</KBtn>
          <KBtn variant="primary" icon="check" @click="showCreate = false">Create agent</KBtn>
        </div>
      </div>
    </template>
  </div>
</template>
