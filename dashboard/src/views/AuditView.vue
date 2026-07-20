<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusPanel from '@/components/StatusPanel.vue'
import { safeAuditMetadata } from '@/lib/analytics'
import { consoleContext, currentRange } from '@/lib/context'
import { dateTimeAttribute, formatTime, humanize, shortId } from '@/lib/format'
import { kernel } from '@/lib/kernel'
import type { AuditEvent, TimeRange } from '@/lib/types'

const events = ref<AuditEvent[]>([])
const eventKind = ref('')
const nextPageToken = ref('')
const loadedRange = ref<TimeRange>()
const loading = ref(false)
const loadingMore = ref(false)
const error = ref<Error>()
let generation = 0

async function load(reset = true): Promise<void> {
  const current = ++generation
  const isMore = !reset
  if (reset) {
    events.value = []
    nextPageToken.value = ''
    loadedRange.value = undefined
    loadingMore.value = false
  }
  if (isMore) loadingMore.value = true
  else loading.value = true
  error.value = undefined
  try {
    const range = reset || !loadedRange.value ? currentRange() : loadedRange.value
    const page = await kernel.queryAuditEvents(range, {
      eventKind: eventKind.value.trim() || undefined,
      pageToken: isMore ? nextPageToken.value : undefined,
    })
    if (current !== generation) return
    events.value = isMore ? [...events.value, ...page.items] : page.items
    nextPageToken.value = page.nextPageToken
    loadedRange.value = range
  } catch (caught) {
    if (current === generation) {
      error.value =
        caught instanceof Error ? caught : new Error('Audit events could not be loaded.')
    }
  } finally {
    if (current === generation) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

onMounted(() => void load())
watch(
  () => consoleContext.range,
  () => void load(),
)
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="Investigate"
      title="Audit ledger"
      description="Immutable control and security evidence. Kave strips credential-like metadata before it reaches this API."
    />

    <form class="filter-bar audit-filter" @submit.prevent="load()">
      <label>
        <span>Event kind</span>
        <input v-model.trim="eventKind" autocomplete="off" placeholder="All events" />
      </label>
      <button type="submit" class="button button-primary" :disabled="loading">Run query</button>
    </form>

    <StatusPanel
      v-if="loading && !events.length"
      title="Loading audit ledger"
      message="Reading immutable security evidence."
      busy
    />
    <StatusPanel
      v-else-if="error && !events.length"
      title="Audit unavailable"
      :message="error.message"
      tone="error"
      @retry="load()"
    >
      <template #action>Try again</template>
    </StatusPanel>

    <section v-else class="panel">
      <header class="panel-header">
        <div>
          <h2>Events</h2>
          <p>Newest first for the selected interval</p>
        </div>
        <span v-if="nextPageToken" class="status-badge warning">Partial</span>
      </header>
      <div v-if="events.length" class="audit-list">
        <article v-for="event in events" :key="event.id" class="audit-row">
          <div class="audit-rail"><span :class="event.outcome" /></div>
          <div class="audit-content">
            <header>
              <div>
                <strong>{{ humanize(event.eventKind) }}</strong
                ><span class="status-badge" :class="event.outcome">{{
                  humanize(event.outcome)
                }}</span>
              </div>
              <time :datetime="dateTimeAttribute(event.createdAtMs)">{{
                formatTime(event.createdAtMs)
              }}</time>
            </header>
            <p>
              <span>{{ humanize(event.actorKind) }}</span>
              <code :title="event.actorId">{{ shortId(event.actorId) }}</code>
              <span>on {{ humanize(event.resourceKind) }}</span>
              <code :title="event.resourceId">{{ shortId(event.resourceId) }}</code>
            </p>
            <dl v-if="safeAuditMetadata(event.metadata).length" class="metadata-list">
              <template v-for="[key, value] in safeAuditMetadata(event.metadata)" :key="key">
                <dt>{{ humanize(key) }}</dt>
                <dd>{{ value }}</dd>
              </template>
            </dl>
          </div>
        </article>
      </div>
      <div v-else class="panel-empty">
        <strong>No audit events</strong><span>No events matched this interval and filter.</span>
      </div>
      <footer v-if="nextPageToken || (error && events.length)" class="panel-footer">
        <span v-if="error" class="form-error" role="alert">{{ error.message }}</span>
        <span v-else>{{ events.length }} events loaded.</span>
        <button
          type="button"
          class="button button-secondary"
          :disabled="loadingMore"
          @click="load(false)"
        >
          {{ loadingMore ? 'Loading…' : 'Load next 200' }}
        </button>
      </footer>
    </section>
  </div>
</template>
