import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import { theme } from './src/theme'
import ui from '@nuxt/ui/vite'

const isProd = process.env.NODE_ENV === 'production'

export default defineConfig({
  plugins: [
    vue(),
    ui({
      ...theme,
      autoImport: {
        dts: true,
        imports: ['vue', '@vueuse/core'],
      },
    }),
    ...(!isProd ? [vueDevTools()] : []),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: '../server/ui/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-vue': ['vue', 'vue-router', '@vueuse/core'],
          'vendor-query': ['@tanstack/vue-query'],
          'vendor-i18n': ['vue-i18n'],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/proxy': 'http://localhost:8080',
    },
  },
})
