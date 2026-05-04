<script setup lang="ts">
import KBadge from './KBadge.vue'
import KStatusDot from './KStatusDot.vue'

const props = defineProps<{ status: string }>()

type Tone = 'success' | 'info' | 'danger' | 'warning' | 'neutral' | 'accent'
const MAP: Record<string, { tone: Tone; label: string }> = {
  completed: { tone: 'success', label: 'completed' },
  running:   { tone: 'info',    label: 'running'   },
  failed:    { tone: 'danger',  label: 'failed'    },
  blocked:   { tone: 'warning', label: 'blocked'   },
  active:    { tone: 'success', label: 'active'    },
  paused:    { tone: 'neutral', label: 'paused'    },
  enabled:   { tone: 'success', label: 'enabled'   },
  available: { tone: 'neutral', label: 'available' },
  ok:        { tone: 'success', label: 'ok'        },
  error:     { tone: 'danger',  label: 'error'     },
  enforce:   { tone: 'accent',  label: 'enforce'   },
  observe:   { tone: 'neutral', label: 'observe'   },
  warn:      { tone: 'warning', label: 'warn'      },
  block:     { tone: 'danger',  label: 'block'     },
}

const m = MAP[props.status] ?? { tone: 'neutral' as Tone, label: props.status }
</script>

<template>
  <KBadge :tone="m.tone">
    <KStatusDot :status="status" />
    {{ m.label }}
  </KBadge>
</template>
