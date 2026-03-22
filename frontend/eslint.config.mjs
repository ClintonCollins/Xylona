import eslint from '@eslint/js'
import { defineConfig } from 'eslint/config'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import prettier from 'eslint-config-prettier'
import globals from 'globals'

export default defineConfig(
  {
    ignores: [
      '**/dist/',
      '**/.quasar/',
      '**/node_modules/',
      '**/src-capacitor/',
      '**/src-cordova/',
      '**/.idea/',
      '**/*.config.*.temporary.compiled*',
      '**/proto/**/*.ts', // Generated protobuf files
      '**/.prettierrc.*', // Prettier config — no lint value
    ],
  },

  // Base JS recommended rules
  eslint.configs.recommended,

  // TypeScript recommended rules for .ts/.tsx/.mts/.vue files
  ...tseslint.configs.recommended.map((config) => ({
    ...config,
    files: ['**/*.ts', '**/*.tsx', '**/*.mts', '**/*.vue'],
  })),

  // Vue recommended rules (bundles vue-eslint-parser internally)
  ...pluginVue.configs['flat/recommended'].map((config) => ({
    ...config,
    files: ['**/*.vue'],
  })),

  // All src files: shared projectService for type-checked linting.
  // IMPORTANT: extraFileExtensions must be identical across all projectService
  // blocks to prevent TypeScript server reloads on every file switch.
  // See: https://typescript-eslint.io/troubleshooting/typed-linting/performance/
  {
    files: ['src/**/*.ts', 'src/**/*.tsx', 'src/**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        ecmaVersion: 'latest',
        sourceType: 'module',
        extraFileExtensions: ['.vue'],
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
      globals: {
        ...globals.browser,
        process: 'readonly', // Vite/Quasar defines process.env at build time
      },
    },
  },

  // Vue files outside src (if any) — parser without type checking
  {
    files: ['**/*.vue'],
    ignores: ['src/**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        ecmaVersion: 'latest',
        sourceType: 'module',
        extraFileExtensions: ['.vue'],
      },
      globals: {
        ...globals.browser,
      },
    },
  },

  // TypeScript files outside src (configs, e2e) — no type-checked rules
  {
    files: ['**/*.ts', '**/*.tsx', '**/*.mts'],
    ignores: ['src/**/*.ts', 'src/**/*.tsx'],
    extends: [tseslint.configs.disableTypeChecked],
    languageOptions: {
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
      },
      globals: {
        ...globals.browser,
        process: 'readonly',
      },
    },
  },

  // Shared TypeScript rule overrides (non-type-checked)
  {
    files: ['**/*.ts', '**/*.tsx', '**/*.mts', '**/*.vue'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      '@typescript-eslint/no-non-null-assertion': 'warn',
    },
  },

  // Type-checked rules — only for src/ where projectService is configured
  {
    files: ['src/**/*.ts', 'src/**/*.tsx', 'src/**/*.vue'],
    ignores: ['src/proto/**'],
    rules: {
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/no-misused-promises': [
        'error',
        { checksVoidReturn: { arguments: false } },
      ],
      '@typescript-eslint/await-thenable': 'error',
    },
  },

  // Vue-specific rule overrides
  {
    files: ['**/*.vue'],
    rules: {
      'vue/multi-word-component-names': 'off',
      'vue/no-v-html': 'warn',
      'vue/require-default-prop': 'off',
      'vue/require-explicit-emits': 'warn',
      'vue/component-name-in-template-casing': ['error', 'kebab-case'],
      'vue/html-self-closing': [
        'error',
        {
          html: {
            void: 'always',
            normal: 'never',
            component: 'always',
          },
          svg: 'always',
          math: 'always',
        },
      ],
    },
  },

  // Node.js globals for config files and E2E tests
  {
    files: ['e2e/**/*.ts', '*.config.ts', '*.config.mjs', 'playwright*.config.ts'],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
  },

  // Prettier must be last to disable conflicting format rules
  prettier,
)
