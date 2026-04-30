import { ref, onUnmounted, type Ref } from 'vue'
import type { Span } from '@/types/api'

const MAX_LIVE_SPANS = 200

export interface UseSpanStreamOptions {
  runId?: string
  actionId?: string
}

export interface UseSpanStreamResult {
  spans: Ref<Span[]>
  isLive: Ref<boolean>
}

export function useSpanStream(options: UseSpanStreamOptions = {}): UseSpanStreamResult {
  const spans = ref<Span[]>([])
  const isLive = ref(false)

  const url = new URL('/api/v1/spans/stream', window.location.origin)
  if (options.runId) url.searchParams.set('run_id', options.runId)
  if (options.actionId) url.searchParams.set('action_id', options.actionId)

  const es = new EventSource(url.toString())

  es.onopen = () => {
    isLive.value = true
  }

  es.onmessage = (e: MessageEvent) => {
    try {
      const span = JSON.parse(e.data) as Span
      spans.value.unshift(span)
      if (spans.value.length > MAX_LIVE_SPANS) {
        spans.value.length = MAX_LIVE_SPANS
      }
    } catch {
      // ignore malformed events
    }
  }

  es.onerror = () => {
    isLive.value = false
  }

  onUnmounted(() => {
    es.close()
    isLive.value = false
  })

  return { spans, isLive }
}
