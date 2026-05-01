import { createConnectTransport } from '@connectrpc/connect-web'

const baseUrl = import.meta.env.VITE_RPC_BASE_URL || `${window.location.origin}/rpc`

export const transport = createConnectTransport({
  baseUrl,
  useBinaryFormat: false,
})
