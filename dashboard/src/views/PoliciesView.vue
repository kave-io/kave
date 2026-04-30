<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import DetailRow from '../components/DetailRow.vue'
import { useAgents, usePolicy } from '@/lib/queries'
import { envId } from '@/stores/workspace'
import { useCurrencyStore } from '@/stores/currency'

const { t } = useI18n()
const currencyStore = useCurrencyStore()

const { data: agents, isLoading } = useAgents(envId)
const selectedPolicyId = ref<string | null>(null)

const policyIds = computed(() => {
  const ids = new Set<string>()
  agents.value?.forEach(a => {
    if (a.policy_id) ids.add(a.policy_id)
  })
  return Array.from(ids)
})

const policyId = computed(() => selectedPolicyId.value || '')
const { data: selectedPolicy } = usePolicy(policyId)

const policiesWithUsage = computed(() => {
  return policyIds.value.map(policyId => {
    const agentCount = (agents.value ?? []).filter(a => a.policy_id === policyId).length
    return { policyId, agentCount }
  })
})

</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <PageHeader :title="t('pages.policies.title')" :subtitle="t('pages.policies.subtitle')" icon="i-lucide-shield" />

    <UCard class="rounded-xl">
      <div v-if="isLoading" class="grid h-32 place-items-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <div v-else-if="policyIds.length === 0" class="grid h-32 place-items-center text-center">
        <div class="space-y-2">
          <UIcon name="i-lucide-shield" class="mx-auto size-8 text-muted" />
          <p class="text-sm font-medium">No policies in use</p>
          <p class="text-xs text-muted">Create and attach policies to agents via the API.</p>
        </div>
      </div>

      <div v-else>
        <div class="divide-y divide-border/50">
          <div
            v-for="item in policiesWithUsage"
            :key="item.policyId"
            class="flex items-center justify-between px-4 py-2.5 cursor-pointer hover:bg-muted/40 transition"
            @click="selectedPolicyId = item.policyId"
          >
            <div class="flex-1 min-w-0">
              <p class="text-sm font-mono truncate">{{ item.policyId }}</p>
            </div>
            <div class="ml-4 text-xs text-muted shrink-0 tabular-nums">
              {{ item.agentCount }} {{ item.agentCount === 1 ? 'agent' : 'agents' }}
            </div>
          </div>
        </div>
      </div>
    </UCard>

    <!-- Policy detail drawer -->
    <USlideover v-model="selectedPolicyId" title="">
      <template v-if="selectedPolicyId" #header>
        <div class="space-y-1">
          <h2 class="text-base font-semibold">Policy</h2>
          <p class="text-xs text-muted font-mono">{{ selectedPolicyId }}</p>
        </div>
      </template>
      <div v-if="selectedPolicyId" class="space-y-6">
        <section v-if="selectedPolicy" class="space-y-3">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted">Restrictions</h3>
          <div class="space-y-3">
            <DetailRow
              v-if="selectedPolicy.budget_cap"
              label="Budget Cap"
              :value="currencyStore.format(selectedPolicy.budget_cap)"
              large
            />
            <div v-if="selectedPolicy.allowed_connectors?.length" class="border border-default/60 rounded-lg px-3 py-2.5">
              <p class="text-xs text-muted mb-2">Allowed Connectors</p>
              <div class="flex flex-wrap gap-1.5">
                <UBadge v-for="c in selectedPolicy.allowed_connectors" :key="c" variant="soft" size="sm">
                  {{ c }}
                </UBadge>
              </div>
            </div>
            <div v-if="selectedPolicy.allowed_methods?.length" class="border border-default/60 rounded-lg px-3 py-2.5">
              <p class="text-xs text-muted mb-2">Allowed Methods</p>
              <div class="flex flex-wrap gap-1.5">
                <UBadge v-for="m in selectedPolicy.allowed_methods" :key="m" variant="soft" size="sm">
                  {{ m }}
                </UBadge>
              </div>
            </div>
          </div>
        </section>

        <section class="border-t border-default pt-4">
          <h3 class="text-xs font-medium uppercase tracking-wide text-muted mb-3">Attached Agents</h3>
          <div v-if="(agents ?? []).filter(a => a.policy_id === selectedPolicyId).length > 0" class="space-y-2">
            <div
              v-for="agent in (agents ?? []).filter(a => a.policy_id === selectedPolicyId)"
              :key="agent.id"
              class="text-sm py-2 px-2.5 rounded hover:bg-muted/30 transition"
            >
              <p class="font-medium">{{ agent.name }}</p>
              <p class="text-xs text-muted font-mono">{{ agent.id }}</p>
            </div>
          </div>
          <div v-else class="text-xs text-muted py-2">
            No agents attached.
          </div>
        </section>
      </div>
    </USlideover>
  </div>
</template>
