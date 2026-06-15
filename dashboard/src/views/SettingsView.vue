<script setup lang="ts">
import { useColorMode } from '@vueuse/core'
import { KIcon, KCard, KCopyBtn } from '@/components/kv'
import LocaleSelector from '@/components/LocaleSelector.vue'
import CurrencySelector from '@/components/CurrencySelector.vue'

const colorMode = useColorMode()

const PROXIES = [
  { k: 'OpenAI Compatible', v: 'http://127.0.0.1:18080/v1' },
  { k: 'GitHub Tool',       v: 'http://127.0.0.1:18080/v1/tools/github' },
]
</script>

<template>
  <div style="padding: 20px 24px; display: flex; flex-direction: column; gap: 16px; max-width: 880px;">
    <div class="page-h"><div><h1>Settings</h1><p>Configure this Kave workspace.</p></div></div>

    <KCard title="Workspace">
      <div class="kv"><div class="k">project</div><div class="v mono">default</div></div>
      <div class="kv"><div class="k">environment</div><div class="v mono">dev</div></div>
      <div class="kv"><div class="k">trust mode</div><div class="v">local · permissive</div></div>
      <div class="kv"><div class="k">daemon version</div><div class="v mono">0.7.2 (commit a8f3d2c)</div></div>
    </KCard>

    <KCard title="Proxy URLs" subtitle="Point your SDKs and tools at these endpoints">
      <div v-for="p in PROXIES" :key="p.k" class="kv">
        <div class="k">{{ p.k }}</div>
        <div class="v mono" style="display: flex; justify-content: space-between; align-items: center; gap: 8px;">
          <span>{{ p.v }}</span><KCopyBtn :value="p.v" />
        </div>
      </div>
    </KCard>

    <KCard title="Preferences">
      <div class="kv">
        <div class="k">
          <div style="font-size: 13px; color: var(--text); font-weight: 500;">Theme</div>
          <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px;">Light or dark surface tones</div>
        </div>
        <div class="v" style="display: flex; justify-content: flex-end;">
          <div style="display: flex; gap: 4px; background: var(--surface-2); padding: 3px; border-radius: 6px;">
            <button
              v-for="m in (['light', 'dark'] as const)"
              :key="m"
              class="btn btn-sm"
              :style="{
                height: '26px', padding: '0 12px',
                background: colorMode === m ? 'var(--surface)' : 'transparent',
                border: '1px solid ' + (colorMode === m ? 'var(--border)' : 'transparent'),
                color: colorMode === m ? 'var(--text)' : 'var(--text-muted)',
              }"
              @click="colorMode = m"
            >
              <KIcon :name="m === 'light' ? 'sun' : 'moon'" :size="13" />{{ m }}
            </button>
          </div>
        </div>
      </div>

      <div class="kv">
        <div class="k">
          <div style="font-size: 13px; color: var(--text); font-weight: 500;">Language</div>
          <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px;">Display language for the dashboard UI</div>
        </div>
        <div class="v" style="display: flex; justify-content: flex-end;"><LocaleSelector /></div>
      </div>

      <div class="kv">
        <div class="k">
          <div style="font-size: 13px; color: var(--text); font-weight: 500;">Currency</div>
          <div style="font-size: 12px; color: var(--text-dim); margin-top: 2px;">Display currency for spend &amp; budgets</div>
        </div>
        <div class="v" style="display: flex; justify-content: flex-end;"><CurrencySelector /></div>
      </div>
    </KCard>
  </div>
</template>
