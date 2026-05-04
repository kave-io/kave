<script setup lang="ts">
import { AUDIT } from '@/data/mock'
import { KBadge, KCard, KStatusBadge } from '@/components/kv'

function actionTone(action: string, result: string) {
  if (result === 'blocked') return 'warning' as const
  if (action.includes('create')) return 'info' as const
  return 'neutral' as const
}
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div><h1>Audit log</h1><p>Who changed what, and what was allowed or blocked.</p></div>
    </div>
    <KCard flush>
      <table class="tbl">
        <thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Resource</th><th>Result</th></tr></thead>
        <tbody>
          <tr v-for="(e, i) in AUDIT" :key="i">
            <td class="mono" style="font-size: 12px; color: var(--text-muted);">{{ new Date(e.ts).toISOString().slice(11, 19) }}</td>
            <td class="mono" style="font-size: 12px;">{{ e.actor }}</td>
            <td><KBadge :tone="actionTone(e.action, e.result)">{{ e.action }}</KBadge></td>
            <td class="mono" style="font-size: 12px;">{{ e.resource }}</td>
            <td><KStatusBadge :status="e.result === 'blocked' ? 'blocked' : 'ok'" /></td>
          </tr>
        </tbody>
      </table>
    </KCard>
  </div>
</template>
