import { createConnectTransport } from '@connectrpc/connect-web'

import { runtimeEnv } from '@/lib/env'

const baseUrl = runtimeEnv.VITE_RPC_BASE_URL || `${window.location.origin}/rpc`

export const transport = createConnectTransport({
  baseUrl,
  useBinaryFormat: false,
})
