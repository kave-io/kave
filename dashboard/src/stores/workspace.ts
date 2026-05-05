import { ref } from 'vue'
import { runtimeEnv } from '@/lib/env'

// Active project and environment — set by the selector in the layout.
// Defaults to 'default' for local daemon mode.
export const projectId = ref<string>(runtimeEnv.VITE_PROJECT_ID || 'default')
export const envId = ref<string>(runtimeEnv.VITE_ENV_ID || 'default')
