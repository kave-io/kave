<script setup lang="ts">
// Policies are fetched by ID only (no list endpoint yet).
// This view is a placeholder until list is added post-v1.
</script>

<template>
  <div class="space-y-6 p-4 lg:p-6">
    <div>
      <h1 class="text-xl font-semibold tracking-tight">Policies</h1>
      <p class="text-sm text-muted mt-0.5">Budget caps, allowed connectors, and guardrail rules.</p>
    </div>

    <UCard class="rounded-xl">
      <div class="grid h-48 place-items-center text-center">
        <div>
          <UIcon name="i-lucide-shield" class="mx-auto size-10 text-muted" />
          <p class="mt-3 font-medium">Policy list coming in v1.1</p>
          <p class="text-sm text-muted mt-1">Use the API to create and attach policies to agents.</p>
        </div>
      </div>
    </UCard>
  </div>
</template>
