import { globalIgnores } from 'eslint/config'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import pluginVue from 'eslint-plugin-vue'
import pluginPlaywright from 'eslint-plugin-playwright'
import pluginVitest from '@vitest/eslint-plugin'
import skipFormatting from 'eslint-config-prettier/flat'

export default defineConfigWithVueTs(
  { name: 'console/source', files: ['**/*.{vue,ts,mts,tsx}'] },
  globalIgnores([
    '**/dist/**',
    '**/coverage/**',
    '**/playwright-report/**',
    '**/test-results/**',
    'src/gen/**',
  ]),
  ...pluginVue.configs['flat/recommended'],
  vueTsConfigs.recommended,
  { ...pluginPlaywright.configs['flat/recommended'], files: ['e2e/**/*.spec.ts'] },
  { ...pluginVitest.configs.recommended, files: ['src/**/__tests__/*.{test,spec}.ts'] },
  skipFormatting,
)
