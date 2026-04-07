<script setup lang="ts">
defineProps<{
  rows: Array<{
    id: string
    agent: string
    model: string
    status: string
    latency: string
    cost: string
    startedAt: string
  }>
}>()

const columns = [
  { accessorKey: 'id', header: 'Trace' },
  { accessorKey: 'agent', header: 'Agent' },
  { accessorKey: 'model', header: 'Model' },
  { accessorKey: 'status', header: 'Status' },
  { accessorKey: 'latency', header: 'Latency' },
  { accessorKey: 'cost', header: 'Cost' },
  { accessorKey: 'startedAt', header: 'Started' },
]
</script>

<template>
  <UCard class="rounded-xl">
    <template #header>
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-base font-semibold">Recent traces</h3>
          <p class="text-sm text-muted">Replayable runs, failures, and spend visibility.</p>
        </div>
        <UButton variant="soft" icon="i-lucide-arrow-right">View all</UButton>
      </div>
    </template>

    <UTable :data="rows" :columns="columns">
      <template #status-cell="{ row }">
        <UBadge
          :color="
            row.original.status === 'success'
              ? 'success'
              : row.original.status === 'blocked'
                ? 'error'
                : 'warning'
          "
          variant="soft"
        >
          {{ row.original.status }}
        </UBadge>
      </template>
    </UTable>
  </UCard>
</template>
