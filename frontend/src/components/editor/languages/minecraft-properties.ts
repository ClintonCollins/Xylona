import * as monaco from 'monaco-editor'

const _sample = `#Minecraft server properties
#Sat May 04 19:04:52 CDT 2024
accepts-transfers=false
allow-flight=false
allow-nether=true
broadcast-console-to-ops=true
broadcast-rcon-to-ops=true
difficulty=easy
enable-command-block=false
enable-jmx-monitoring=false
enable-query=false
enable-rcon=false
enable-status=true
enforce-secure-profile=true
enforce-whitelist=false
entity-broadcast-range-percentage=100
force-gamemode=false
function-permission-level=2
gamemode=survival
generate-structures=true
generator-settings={}
hardcore=false
hide-online-players=false
initial-disabled-packs=
initial-enabled-packs=vanilla
level-name=world
level-seed=
level-type=default
log-ips=true
max-build-height=256
max-chained-neighbor-updates=1000000
max-players=20
max-tick-time=60000
max-world-size=29999984
motd=A Minecraft Server a 111
network-compression-threshold=256
online-mode=true
op-permission-level=4
player-idle-timeout=0
prevent-proxy-connections=false
pvp=true
query.port=25565
rate-limit=0
rcon.password=
rcon.port=25575
region-file-compression=deflate
require-resource-pack=false
resource-pack=
resource-pack-id=
resource-pack-prompt=
resource-pack-sha1=
server-ip=
server-port=25565
simulation-distance=10
snooper-enabled=true
spawn-animals=true
spawn-monsters=true
spawn-npcs=true
spawn-protection=16
sync-chunk-writes=true
text-filtering-config=
texture-pack=
use-native-transport=true
view-distance=10
white-list=false`

export default () => {
  monaco.languages.register({ id: 'minecraft-properties' })

  monaco.languages.setLanguageConfiguration('minecraft-properties', {
    comments: {
      lineComment: '#',
    },
    brackets: [['{', '}']],
    autoClosingPairs: [{ open: '{', close: '}' }],
  })
  // Register a tokens provider for the language
  monaco.languages.setMonarchTokensProvider('minecraft-properties', {
    defaultToken: '',
    tokenPostfix: '.ini',

    // we include these common regular expressions
    escapes: /\\(?:[abfnrtv\\"']|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})/,
    keywords: ['true', 'false'],
    operators: ['='],
    tokenizer: {
      root: [
        // sections
        [/^\[[^\]]*\]/, 'metatag'],
        [/^.+(?==)/gm, 'keyword'],
        // keys
        // [/([^\n=]*)(=)(true|false|)([^0-9]*)/, ['key', 'delimiter', 'keyword', 'string']],
        // whitespace
        { include: '@whitespace' },

        // numbers
        // [/\d+/, 'number'],

        // strings: recover on non-terminated strings
        [/"([^"\\]|\\.)*$/, 'string.invalid'], // non-teminated string
        [/'([^'\\]|\\.)*$/, 'string.invalid'], // non-teminated string
        [/"/, 'string', '@string."'],
        [/'/, 'string', "@string.'"],
        // [/\d+(_+\d+)*/, 'number']
        // [/^.+(?==)/gmi, 'keyword'],
        // [/=(.+$)/gmi, {
        //     cases: {
        //         '@keywords': '$1',
        //         '@default': 'string'
        //     }
        // }],
        // [/[a-z_$][\w$]*/, {
        // cases: {
        //     '@keywords': 'comment',
        //     '@default': 'string'
        // }
        // }],
      ],
      whitespace: [
        [/[ \t\r\n]+/, ''],
        [/^\s*[#;].*$/, 'comment'],
      ],

      string: [
        [/[^\\"']+/, 'string'],
        [/@escapes/, 'string.escape'],
        [/\\./, 'string.escape.invalid'],
        [
          /["']/,
          {
            cases: {
              '$#==$S2': { token: 'string', next: '@pop' },
              '@default': 'string',
            },
          },
        ],
      ],
    },
  })

  // Define a new theme that contains only rules that match this language
  monaco.editor.defineTheme('myCoolTheme', {
    base: 'vs-dark',
    inherit: false,
    rules: [
      { token: 'custom-info', foreground: '808080' },
      { token: 'custom-error', foreground: 'ff0000', fontStyle: 'bold' },
      { token: 'custom-notice', foreground: 'FFA500' },
      { token: 'custom-date', foreground: '008800' },
    ],
    colors: {
      'editor.foreground': '#e0e4e6',
    },
  })
  const SERVER_PROPERTY_KEYWORDS = [
    'accepts-transfers',
    'allow-flight',
    'allow-nether',
    'broadcast-console-to-ops',
    'broadcast-rcon-to-ops',
    'difficulty',
    'enable-command-block',
    'enable-jmx-monitoring',
    'enable-query',
    'enable-rcon',
    'enable-status',
    'enforce-secure-profile',
    'enforce-whitelist',
    'entity-broadcast-range-percentage',
    'force-gamemode',
    'function-permission-level',
    'gamemode',
    'generate-structures',
    'generator-settings',
    'hardcore',
    'hide-online-players',
    'initial-disabled-packs',
    'initial-enabled-packs',
    'level-name',
    'level-seed',
    'level-type',
    'log-ips',
    'max-build-height',
    'max-chained-neighbor-updates',
    'max-players',
    'max-tick-time',
    'max-world-size',
    'motd',
    'network-compression-threshold',
    'online-mode',
    'op-permission-level',
    'player-idle-timeout',
    'prevent-proxy-connections',
    'pvp',
    'query.port',
    'rate-limit',
    'rcon.password',
    'rcon.port',
    'region-file-compression',
    'require-resource-pack',
    'resource-pack',
    'resource-pack-id',
    'resource-pack-prompt',
    'resource-pack-sha1',
    'server-ip',
    'server-port',
    'simulation-distance',
    'snooper-enabled',
    'spawn-animals',
    'spawn-monsters',
    'spawn-npcs',
    'spawn-protection',
    'sync-chunk-writes',
    'text-filtering-config',
    'texture-pack',
    'use-native-transport',
    'view-distance',
    'white-list',
  ]
  // Register a completion item provider for the new language
  monaco.languages.registerCompletionItemProvider('minecraft-properties', {
    provideCompletionItems: (model, position) => {
      const word = model.getWordUntilPosition(position)
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      }
      const suggestions = SERVER_PROPERTY_KEYWORDS.map((keyword) => ({
        label: keyword,
        kind: monaco.languages.CompletionItemKind.Snippet,
        insertText: `${keyword}=`,
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range,
      }))
      suggestions.push({
        label: 'true',
        kind: monaco.languages.CompletionItemKind.Text,
        insertText: 'true',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range,
      })
      return { suggestions: suggestions }
    },
  })
}
