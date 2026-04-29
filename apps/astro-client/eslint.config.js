// For more info, see https://github.com/storybookjs/eslint-plugin-storybook#configuration-flat-config-format
import storybook from "eslint-plugin-storybook";

import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'
import localTheme from './eslint-rules/index.js'

export default defineConfig([
  globalIgnores(['dist', '.react-router', 'build', 'playwright-report', 'test-results']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/purity': 'off',
      'react-hooks/immutability': 'off',
    },
  },
  // Custom theme rule: enforces semantic tokens / `<Card>` over raw palette
  // utilities in component code. See eslint-rules/no-raw-theme-colors.js.
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { 'local-theme': localTheme },
    rules: {
      'local-theme/no-raw-theme-colors': 'error',
    },
  },
  // Allowlist: stories, tests, and intentionally-literal UI primitives.
  {
    files: [
      'src/stories/**',
      'src/**/*.test.{ts,tsx}',
      'src/components/ui/switch.tsx',
      'src/components/ui/image-cropper.tsx',
    ],
    rules: {
      'local-theme/no-raw-theme-colors': 'off',
    },
  },
  ...storybook.configs["flat/recommended"],
  {
    files: ['src/stories/**'],
    rules: {
      'storybook/no-redundant-story-name': 'off',
    },
  },
])
