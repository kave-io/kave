<script setup lang="ts">
import { ref, computed } from 'vue'
import { CONNECTORS, type Connector } from '@/data/mock'
import { KIcon, KBtn, KBadge, KStatusBadge, KCopyBtn, KEmptyState, KLiveStream } from '@/components/kv'

const selected = ref<Connector | null>(null)
const tab = ref<'setup' | 'methods' | 'credentials' | 'traffic' | 'raw'>('setup')
const tabFilter = ref('All')

const groups = computed(() => ['All', ...CONNECTORS.map(g => g.kind)])
const visibleGroups = computed(() => CONNECTORS.filter(g => tabFilter.value === 'All' || g.kind === tabFilter.value))

const SNIPPETS: Record<string, string> = {
  openai: `export OPENAI_BASE_URL=http://127.0.0.1:18080/v1/openai\nexport OPENAI_API_KEY=<kave-agent-token>`,
  anthropic: `export ANTHROPIC_BASE_URL=http://127.0.0.1:18080/v1/anthropic\nexport ANTHROPIC_API_KEY=<kave-agent-token>`,
  'claude-code': `export ANTHROPIC_BASE_URL=http://127.0.0.1:18080/frameworks/claude-code/anthropic\nexport ANTHROPIC_API_KEY=<kave-agent-token>`,
  ollama: `export OLLAMA_HOST=http://127.0.0.1:18080/frameworks/claude-code/ollama`,
}
const snippet = computed(() => selected.value
  ? (SNIPPETS[selected.value.id] || `# Setup snippets coming soon for ${selected.value.name}.`)
  : '')
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px;">
    <div class="page-h">
      <div>
        <h1>Connectors</h1>
        <p>Frameworks, providers, protocols, and tools Kave can intercept or call.</p>
      </div>
    </div>

    <div style="display: flex; gap: 4px;">
      <button
        v-for="g in groups"
        :key="g"
        class="btn btn-sm"
        :style="{
          background: tabFilter === g ? 'var(--secondary-tint)' : 'transparent',
          color: tabFilter === g ? 'var(--text)' : 'var(--text-muted)',
          border: '1px solid ' + (tabFilter === g ? 'var(--secondary-edge)' : 'transparent'),
        }"
        @click="tabFilter = g"
      >{{ g }}</button>
    </div>

    <section v-for="g in visibleGroups" :key="g.kind">
      <div class="sh" style="margin-bottom: 10px;">{{ g.kind }}</div>
      <div class="conn-grid">
        <div v-for="c in g.items" :key="c.id" class="conn-card" @click="selected = c; tab = 'setup'">
          <div class="top">
            <div class="ico">{{ c.name.slice(0, 2).toUpperCase() }}</div>
            <div style="flex: 1; min-width: 0;">
              <div class="name">{{ c.name }}</div>
              <div class="mono" style="font-size: 11px; color: var(--text-faint);">{{ c.id }}</div>
            </div>
            <KStatusBadge :status="c.status" />
          </div>
          <div class="desc">{{ c.desc }}</div>
          <div style="display: flex; gap: 12px; font-size: 11px; color: var(--text-dim); font-family: var(--font-mono); border-top: 1px solid var(--border-soft); padding-top: 8px;">
            <span>{{ c.methods }} methods</span>
            <span>{{ c.credentials }} cred</span>
            <span style="margin-left: auto;">{{ c.traffic24h }} req/24h</span>
          </div>
        </div>
      </div>
    </section>

    <template v-if="selected">
      <div class="drawer-overlay" @click="selected = null" />
      <aside class="drawer">
        <header class="drawer-h">
          <div class="ico" style="width: 32px; height: 32px; border-radius: 7px; background: var(--surface-2); border: 1px solid var(--border); display: grid; place-items: center; font-family: var(--font-mono); font-size: 13px; font-weight: 600;">{{ selected.name.slice(0, 2).toUpperCase() }}</div>
          <div style="flex: 1;">
            <div class="title">{{ selected.name }}</div>
            <div style="font-size: 12px; color: var(--text-dim);">{{ selected.desc }}</div>
          </div>
          <KStatusBadge :status="selected.status" />
          <KBtn variant="ghost" icon="x" aria-label="Close" @click="selected = null" />
        </header>
        <div class="tabs">
          <button v-for="t in (['setup','methods','credentials','traffic','raw'] as const)" :key="t" :class="['tab', tab === t ? 'active' : '']" @click="tab = t">{{ t }}</button>
        </div>
        <div class="drawer-b">
          <div v-if="tab === 'setup'" style="padding: 16px;">
            <div style="font-size: 13px; color: var(--text-muted); margin-bottom: 10px;">Drop these into your shell or framework config:</div>
            <div class="code" style="position: relative;">{{ snippet }}<div style="position: absolute; top: 8px; right: 8px;"><KCopyBtn :value="snippet" /></div></div>
            <div style="margin-top: 14px; font-size: 12px; color: var(--text-dim);">Tokens are issued per-agent on the Agents page.</div>
          </div>

          <table v-else-if="tab === 'methods'" class="tbl" style="margin: 0;">
            <thead><tr><th>Method</th><th>Verb</th><th>Allowed by default</th></tr></thead>
            <tbody>
              <tr v-for="m in ['chat.completions','embeddings','models.list','audio.transcriptions']" :key="m">
                <td class="mono" style="font-size: 12px;">{{ m }}</td>
                <td class="mono" style="font-size: 12px;">POST</td>
                <td><KBadge tone="success">allow</KBadge></td>
              </tr>
            </tbody>
          </table>

          <div v-else-if="tab === 'credentials'" style="padding: 16px;">
            <table v-if="selected.credentials > 0" class="tbl">
              <tbody>
                <tr>
                  <td class="mono" style="font-size: 12px;">cred_{{ selected.id }}_main</td>
                  <td>•••• kEy42b8</td>
                  <td>3d ago</td>
                  <td><KStatusBadge status="active" /></td>
                </tr>
              </tbody>
            </table>
            <KEmptyState v-else icon="key" title="No credentials">
              This connector requires credentials before agents can call it.
              <template #action>
                <KBtn variant="primary" size="sm" icon="plus">Add credential</KBtn>
              </template>
            </KEmptyState>
          </div>

          <div v-else-if="tab === 'traffic'" style="padding: 16px;">
            <KLiveStream :limit="10" compact />
          </div>

          <pre v-else-if="tab === 'raw'" class="code" style="margin: 16px;">{{ JSON.stringify(selected, null, 2) }}</pre>
        </div>
      </aside>
    </template>
  </div>
</template>
