import { type Config } from 'prettier'

const config: Config = {
  semi: false,
  singleQuote: true,
  trailingComma: 'all',
  printWidth: 100,
  tabWidth: 2,
  useTabs: false,
  bracketSpacing: true,
  arrowParens: 'always',
  endOfLine: 'lf',
  htmlWhitespaceSensitivity: 'css',
  vueIndentScriptAndStyle: false,
}

export default config
