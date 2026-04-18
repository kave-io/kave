import { ref } from 'vue'

// Active project and environment — set by the selector in the layout.
// Defaults to 'default' for both.
export const projectId = ref<string>(
  import.meta.env.VITE_PROJECT_ID || 'default'
)

export const envId = ref<string>(
  import.meta.env.VITE_ENV_ID || 'default'
)
