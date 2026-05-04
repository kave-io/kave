<script setup lang="ts">
import { AGENTS, POLICIES, fmtMoney } from '@/data/mock'
import { KIcon, KBtn, KCard, KStatusBadge, KSparkline, KKv } from '@/components/kv'

const currency = 'USD'
function pctClass(pct: number) { return pct > 0.9 ? 'danger' : pct > 0.7 ? 'warn' : '' }
function policyOf(id: string) { return POLICIES.find(p => p.id === id) }
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div>
        <h1>Cost &amp; Budgets</h1>
        <p>Where money goes, and what limits are enforced.</p>
      </div>
      <div class="toolbar">
        <KBtn icon="rotate" size="sm">Refresh FX</KBtn>
        <KBtn icon="file-text" size="sm">Export</KBtn>
      </div>
    </div>

    <section :style="{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(190px,1fr))', gap: '10px' }">
      <div class="stat">
        <div class="label">Total spend (mo)</div>
        <div class="val">{{ fmtMoney(141.65, currency) }}</div>
        <div class="delta up">+24% vs last mo</div>
        <KSparkline :data="[2,3,4,3,5,7,6,8,11,12,14,18]" />
      </div>
      <div class="stat">
        <div class="label">Forecast (mo)</div>
        <div class="val">{{ fmtMoney(312.40, currency) }}</div>
        <div class="delta">based on 14d avg</div>
        <KSparkline :data="[3,5,7,9,11,14,17,21,25,28,30,32]" color="var(--secondary)" />
      </div>
      <div class="stat">
        <div class="label">Blocked by budget</div>
        <div class="val">7</div>
        <div class="delta">3 agents</div>
      </div>
      <div class="stat">
        <div class="label">Top spender</div>
        <div class="val" style="font-size: 16px;">codegen-agent</div>
        <div class="delta">{{ fmtMoney(73.51, currency) }}</div>
      </div>
    </section>

    <KCard title="Budget usage" subtitle="Per-agent caps and current consumption" flush>
      <table class="tbl">
        <thead><tr>
          <th>Agent</th><th>Policy</th>
          <th class="num">Cap</th><th class="num">Spent</th><th class="num">Remaining</th>
          <th>Behavior</th><th style="width: 200px;">Usage</th>
        </tr></thead>
        <tbody>
          <tr v-for="a in AGENTS" :key="a.id">
            <td>{{ a.name }}</td>
            <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ a.policy }}</td>
            <td class="num mono" style="font-size: 12px;">{{ fmtMoney(a.budget, currency) }}</td>
            <td class="num mono" style="font-size: 12px;">{{ fmtMoney(a.spent, currency) }}</td>
            <td class="num mono" style="font-size: 12px;">{{ fmtMoney(a.budget - a.spent, currency) }}</td>
            <td><KStatusBadge :status="policyOf(a.policy)?.behavior || 'warn'" /></td>
            <td>
              <div style="display: flex; align-items: center; gap: 8px;">
                <div class="prog" style="flex: 1;">
                  <div :class="['fill', pctClass(a.spent / a.budget)]" :style="{ width: Math.min(a.spent / a.budget * 100, 100) + '%' }" />
                </div>
                <span style="font-size: 11px; color: var(--text-muted); width: 36px; text-align: right;">{{ Math.round(a.spent / a.budget * 100) }}%</span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </KCard>

    <section :style="{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: '16px' }">
      <KCard title="Price book" subtitle="Active version: kave/2026-04-01">
        <KKv k="last refreshed" mono>3 days ago</KKv>
        <KKv k="providers covered" mono>openai, anthropic, gemini, groq, ollama</KKv>
        <KKv k="missing prices" mono>0</KKv>
        <div style="display: flex; gap: 6px; margin-top: 12px;">
          <KBtn size="sm" icon="file-text">Edit JSON</KBtn>
          <KBtn size="sm" variant="ghost" icon="rotate">Refresh from kave.io</KBtn>
        </div>
      </KCard>
      <KCard title="FX rates" subtitle="Currency conversion for non-USD providers">
        <KKv k="display" mono>{{ currency }}</KKv>
        <KKv k="USD → IRT" mono>60,000 (operator-set, 2h ago)</KKv>
        <KKv k="USD → EUR" mono>0.92 (auto, 8m ago)</KKv>
        <div style="margin-top: 10px; padding: 10px; background: var(--warning-tint); border-radius: 6px; font-size: 12px; color: var(--warning); display: flex; gap: 8px;">
          <KIcon name="circle-alert" :size="14" :style="{ flexShrink: 0, marginTop: '2px' }" />
          <span>IRT rate is operator-set. Refresh weekly to keep cost figures accurate.</span>
        </div>
      </KCard>
    </section>
  </div>
</template>
