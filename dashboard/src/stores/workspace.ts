import { ref } from 'vue'

// Active workspace ID — set by the workspace selector in the layout.
// Defaults to the VITE_WORKSPACE_ID env var, or 'default' for self-hosted.
export const workspaceId = ref<string>(
  import.meta.env.VITE_WORKSPACE_ID || 'default'
)
