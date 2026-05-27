/// <reference types="vite/client" />
declare module '*.vue'

interface ImportMetaEnv {
  readonly VITE_RPC_BASE_URL?: string
  readonly VITE_PROJECT_ID?: string
  readonly VITE_ENV_ID?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
