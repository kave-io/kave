/// <reference types="vite/client" />
declare module '*.vue'

interface ImportMetaEnv {
  readonly VITE_KERNEL_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
