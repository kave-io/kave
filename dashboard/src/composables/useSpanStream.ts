import { onUnmounted, ref, type Ref } from 'vue'
import type { Span } from '@/types/api'
import { spansClient } from '@/lib/rpc/clients'
import { envId, projectId } from '@/stores/workspace'

const MAX_LIVE_SPANS = 200

export interface UseSpanStreamOptions {
  runId?: string
  actionId?: string
}

export interface UseSpanStreamResult {
  spans: Ref<Span[]>
  isLive: Ref<boolean>
  error: Ref<string | null>
}

export function useSpanStream(options: UseSpanStreamOptions = {}): UseSpanStreamResult {
  const spans = ref<Span[]>([])
  const isLive = ref(false)
  const error = ref<string | null>(null)
  const controller = new AbortController()

  void (async () => {
    try {
      const stream = await spansClient.stream(
        { projectId: projectId.value, envId: envId.value, runId: options.runId },
        controller.signal,
      )
      isLive.value = true
      for await (const span of stream) {
        if (options.actionId && span.action_id !== options.actionId) continue
        spans.value.unshift(span)
        if (spans.value.length > MAX_LIVE_SPANS) spans.value.length = MAX_LIVE_SPANS
      }
    } catch (err) {
      if (!controller.signal.aborted) error.value = err instanceof Error ? err.message : 'Span stream disconnected'
    } finally {
      isLive.value = false
    }
  })()

  onUnmounted(() => {
    controller.abort()
    isLive.value = false
  })

  return { spans, isLive, error }
}
