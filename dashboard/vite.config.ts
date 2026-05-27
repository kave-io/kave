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
      // Keep generated types within this repo so Docker builds don't depend on sibling dirs.
      '@/gen': fileURLToPath(new URL('./src/gen', import.meta.url)),
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: '../server/ui/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 450,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('@iconify') || id.includes('@unhead')) return 'vendor-ui-runtime'
          if (id.includes('@nuxt/ui')) return 'vendor-ui'
          if (id.includes('@tanstack/vue-query')) return 'vendor-query'
          if (id.includes('vue-i18n')) return 'vendor-i18n'
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
